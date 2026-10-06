package reconciler

import (
	"context"
	"errors"
	"fmt"
	orcadocker "github.com/swapnil404/orca/agent/internal/docker"
	"github.com/swapnil404/orca/agent/internal/postgres"
	"log/slog"
)

type primaryConfigDockerClient interface {
	DockerClient
	WriteConfig(context.Context, string, *orcadocker.ConfigMount) error
	ExecContainer(context.Context, string, []string) (string, error)
}

type postgresNodeUpdate struct {
	containerID string
	dataPath    string
	configPath  string
	appliedPath string
	applied     map[string]string
}

func createPrimary(ctx context.Context, docker DockerClient, spec orcadocker.ContainerSpec, params map[string]string) error {
	credentials, ok := docker.(clusterCredentialDockerClient)
	if !ok {
		return errors.New("docker client does not support cluster credentials")
	}
	password, err := credentials.EnsureClusterPassword(ctx, spec.ClusterID)
	if err != nil {
		return err
	}
	if err := credentials.WriteConfig(ctx, spec.ClusterID, &orcadocker.ConfigMount{
		RelativePath: "postgres/password", Content: password + "\n", Mode: 0o600,
	}); err != nil {
		return fmt.Errorf("write PostgreSQL password file: %w", err)
	}
	spec.Env = append(spec.Env, "POSTGRES_PASSWORD_FILE=/etc/orca/password")
	baseline, err := postgres.RenderConfig(spec.ClusterID, nil)
	if err != nil {
		return err
	}
	spec.Config.Content = baseline
	containerID, err := docker.CreateContainer(ctx, spec)
	if err != nil {
		return err
	}
	if err := docker.StartContainer(ctx, containerID); err != nil {
		return err
	}
	configDocker, ok := docker.(primaryConfigDockerClient)
	if !ok {
		return errors.New("docker client does not support PostgreSQL configuration updates")
	}
	if err := postgres.WaitForPrimaryReady(ctx, configDocker, containerID); err != nil {
		return err
	}
	if err := applyPostgresConfig(ctx, configDocker, docker, containerID, spec.ClusterID, orcadocker.VolumeMountPath(spec.ClusterID)+"/primary", orcadocker.PostgresConfigRelativePath, nil, params); err != nil {
		return err
	}
	if err := writeAppliedParams(ctx, configDocker, spec.ClusterID, orcadocker.PostgresAppliedConfigRelativePath, orcadocker.VolumeMountPath(spec.ClusterID)+"/primary", params); err != nil {
		return err
	}
	return synchronizePostgresPassword(ctx, credentials, containerID, password)
}

func recoverPrimary(ctx context.Context, docker DockerClient, containerID string) error {
	configDocker, ok := docker.(primaryConfigDockerClient)
	if !ok {
		return errors.New("docker client does not support PostgreSQL readiness checks")
	}
	if err := docker.StartContainer(ctx, containerID); err != nil {
		return err
	}
	return postgres.WaitForPrimaryReady(ctx, configDocker, containerID)
}

func updatePrimary(ctx context.Context, docker DockerClient, action Action) error {
	update, ok := action.Spec.(*primaryUpdateSpec)
	if !ok || update.Desired == nil || update.Actual == nil || update.Actual.ContainerId == "" {
		return errors.New("update_primary action requires desired and actual primary state")
	}
	spec, err := primaryContainerSpec(action)
	if err != nil {
		return err
	}

	if primaryRequiresReplacement(update.Desired, update.Actual) {
		if err := stopAndRemove(ctx, docker, update.Actual.ContainerId); err != nil {
			return fmt.Errorf("remove existing primary: %w", err)
		}
		return createPrimary(ctx, docker, spec, update.Desired.Params)
	}

	configDocker, ok := docker.(primaryConfigDockerClient)
	if !ok {
		return errors.New("docker client does not support PostgreSQL configuration updates")
	}
	if update.Actual.Status != "running" {
		if err := docker.StartContainer(ctx, update.Actual.ContainerId); err != nil {
			return err
		}
		if err := postgres.WaitForPrimaryReady(ctx, configDocker, update.Actual.ContainerId); err != nil {
			return err
		}
	}
	nodes := []postgresNodeUpdate{{
		containerID: update.Actual.ContainerId,
		dataPath:    orcadocker.VolumeMountPath(action.ClusterID) + "/primary",
		configPath:  orcadocker.PostgresConfigRelativePath,
		appliedPath: orcadocker.PostgresAppliedConfigRelativePath,
		applied:     update.Actual.AppliedParams,
	}}
	for _, replica := range update.Actual.Replicas {
		if replica == nil || replica.ContainerId == "" {
			continue
		}
		if replica.AppliedParams == nil && len(update.Desired.Params) > 0 {
			continue
		}
		identity, err := postgres.DeriveReplicaIdentity(action.ClusterID, replica.Id)
		if err != nil {
			return err
		}
		nodes = append(nodes, postgresNodeUpdate{
			containerID: replica.ContainerId, dataPath: identity.DataPath,
			configPath:  orcadocker.PostgresReplicaConfigRelativePath(replica.Id),
			appliedPath: orcadocker.PostgresReplicaAppliedConfigRelativePath(replica.Id),
			applied:     replica.AppliedParams,
		})
	}
	for _, node := range nodes {
		if err := preflightPostgresConfig(ctx, configDocker, node.containerID, node.dataPath, node.applied, update.Desired.Params); err != nil {
			return err
		}
	}
	for index, node := range nodes {
		if err := applyPostgresConfig(ctx, configDocker, docker, node.containerID, action.ClusterID, node.dataPath, node.configPath, node.applied, update.Desired.Params); err != nil {
			rollbackErr := rollbackPostgresNodes(context.WithoutCancel(ctx), configDocker, docker, action.ClusterID, nodes[:index+1])
			return errors.Join(err, rollbackErr)
		}
	}
	for _, node := range nodes {
		if err := writeAppliedParams(ctx, configDocker, action.ClusterID, node.appliedPath, node.dataPath, update.Desired.Params); err != nil {
			rollbackErr := rollbackPostgresNodes(context.WithoutCancel(ctx), configDocker, docker, action.ClusterID, nodes)
			return errors.Join(err, rollbackErr)
		}
	}
	return nil
}

func preflightPostgresConfig(ctx context.Context, configDocker primaryConfigDockerClient, containerID, dataPath string, applied, desired map[string]string) error {
	changed := postgres.ChangedParameters(desired, applied)
	metadata, err := postgres.InspectParameters(ctx, configDocker, containerID, changed)
	if err != nil {
		return err
	}
	if _, err := postgres.ClassifyConfigUpdate(changed, metadata); err != nil {
		return err
	}
	return postgres.ValidateParameterValues(ctx, configDocker, containerID, dataPath, desired)
}

func rollbackPostgresNodes(ctx context.Context, configDocker primaryConfigDockerClient, docker DockerClient, clusterID string, nodes []postgresNodeUpdate) error {
	var rollbackErr error
	for index := len(nodes) - 1; index >= 0; index-- {
		node := nodes[index]
		rollbackErr = errors.Join(rollbackErr, rollbackPostgresNode(ctx, configDocker, docker, node.containerID, clusterID, node.dataPath, node.configPath, node.applied))
		rollbackErr = errors.Join(rollbackErr, writeAppliedParams(ctx, configDocker, clusterID, node.appliedPath, node.dataPath, node.applied))
	}
	return rollbackErr
}

func rollbackPostgresNode(ctx context.Context, configDocker primaryConfigDockerClient, docker DockerClient, containerID, clusterID, dataPath, relativePath string, applied map[string]string) error {
	config, err := postgres.RenderNodeConfig(clusterID, dataPath, applied)
	if err != nil {
		return err
	}
	writeErr := configDocker.WriteConfig(ctx, clusterID, &orcadocker.ConfigMount{RelativePath: relativePath, ContainerPath: orcadocker.PostgresConfigContainerPath, Content: config})
	if writeErr != nil {
		return writeErr
	}
	stopErr := docker.StopContainer(ctx, containerID)
	startErr := docker.StartContainer(ctx, containerID)
	if stopErr != nil || startErr != nil {
		return errors.Join(stopErr, startErr)
	}
	return postgres.WaitForConfigApplied(ctx, configDocker, containerID, len(applied))
}

func applyPostgresConfig(ctx context.Context, configDocker primaryConfigDockerClient, docker DockerClient, containerID, clusterID, dataPath, relativePath string, applied, desired map[string]string) error {
	changed := postgres.ChangedParameters(desired, applied)
	metadata, err := postgres.InspectParameters(ctx, configDocker, containerID, changed)
	if err != nil {
		return err
	}
	method, err := postgres.ClassifyConfigUpdate(changed, metadata)
	if err != nil {
		return err
	}
	if err := postgres.ValidateParameterValues(ctx, configDocker, containerID, dataPath, desired); err != nil {
		return err
	}
	config, err := postgres.RenderNodeConfig(clusterID, dataPath, desired)
	if err != nil {
		return err
	}
	if err := configDocker.WriteConfig(ctx, clusterID, &orcadocker.ConfigMount{RelativePath: relativePath, ContainerPath: orcadocker.PostgresConfigContainerPath, Content: config}); err != nil {
		return fmt.Errorf("write PostgreSQL config: %w", err)
	}
	if len(changed) > 0 {
		switch method {
		case postgres.ConfigUpdateReload:
			slog.Info("reloading PostgreSQL configuration", "cluster_id", clusterID, "container_id", containerID)
			if _, err := configDocker.ExecContainer(ctx, containerID, []string{"psql", "--username", "postgres", "--dbname", "postgres", "--tuples-only", "--no-align", "--command", "SELECT pg_reload_conf();"}); err != nil {
				return fmt.Errorf("reload PostgreSQL config: %w", err)
			}
		case postgres.ConfigUpdateRestart:
			slog.Info("restarting PostgreSQL for configuration change", "cluster_id", clusterID, "container_id", containerID)
			if err := docker.StopContainer(ctx, containerID); err != nil {
				return err
			}
			if err := docker.StartContainer(ctx, containerID); err != nil {
				return err
			}
		default:
			return errors.New("unknown PostgreSQL config update method")
		}
	}
	return postgres.WaitForConfigApplied(ctx, configDocker, containerID, len(desired))
}

func writeAppliedParams(ctx context.Context, configDocker interface {
	WriteConfig(context.Context, string, *orcadocker.ConfigMount) error
}, clusterID, relativePath, dataPath string, params map[string]string) error {
	config, err := postgres.RenderNodeConfig(clusterID, dataPath, params)
	if err != nil {
		return err
	}
	return configDocker.WriteConfig(ctx, clusterID, &orcadocker.ConfigMount{RelativePath: relativePath, Content: config})
}

func primaryContainerSpec(action Action) (orcadocker.ContainerSpec, error) {
	if spec, ok := action.Spec.(orcadocker.ContainerSpec); ok {
		return spec, nil
	}

	cluster, ok := action.Spec.(*ClusterSpec)
	if update, updateOK := action.Spec.(*primaryUpdateSpec); updateOK {
		cluster, ok = update.Desired, update.Desired != nil
	}
	if !ok {
		return orcadocker.ContainerSpec{}, fmt.Errorf("%s action requires ClusterSpec", action.Type)
	}

	config, err := postgres.RenderConfig(cluster.Id, cluster.Params)
	if err != nil {
		return orcadocker.ContainerSpec{}, err
	}
	spec := orcadocker.ContainerSpec{
		ClusterID: cluster.Id,
		Kind:      orcadocker.ContainerKindPrimary,
		Image:     primaryImage(cluster),
		Env: []string{
			"POSTGRES_HOST_AUTH_METHOD=reject",
			"PGDATA=" + orcadocker.VolumeMountPath(cluster.Id) + "/primary",
		},
		Command:   []string{"postgres", "-c", "config_file=" + orcadocker.PostgresConfigContainerPath},
		UseVolume: true,
		Config: &orcadocker.ConfigMount{
			RelativePath: orcadocker.PostgresConfigRelativePath, ContainerPath: orcadocker.PostgresConfigContainerPath, Content: config,
		},
	}
	if cluster.PgBackRest != nil {
		spec.Binds = []orcadocker.BindMount{{Source: cluster.PgBackRest.RepoPath, Path: cluster.PgBackRest.RepoPath, Create: true}}
	}
	return spec, nil
}

func primaryContainerID(action Action) (string, error) {
	cluster, ok := action.Spec.(*ActualCluster)
	if !ok {
		return "", fmt.Errorf("%s action requires ActualCluster", action.Type)
	}

	return cluster.ContainerId, nil
}

func postgresImage(version string) string {
	if version == "" {
		return "postgres:latest"
	}

	return "postgres:" + version
}

func primaryImage(cluster *ClusterSpec) string {
	if cluster.PgBackRest == nil {
		return postgresImage(cluster.Version)
	}
	if cluster.Version == "" {
		return "orca-postgres:latest"
	}
	return "orca-postgres:" + cluster.Version
}
