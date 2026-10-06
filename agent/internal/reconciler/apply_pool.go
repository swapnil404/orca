package reconciler

import (
	"context"
	"errors"
	"fmt"
	orcadocker "github.com/swapnil404/orca/agent/internal/docker"
	"github.com/swapnil404/orca/agent/internal/pgbouncer"
	"log/slog"
	"strings"
)

type pgBouncerDockerClient interface {
	DockerClient
	pgbouncer.ConsoleExecutor
	WriteConfig(ctx context.Context, clusterID string, config *orcadocker.ConfigMount) error
}

func updatePgBouncer(ctx context.Context, docker DockerClient, action Action) error {
	update, ok := action.Spec.(*pgBouncerUpdateSpec)
	if !ok || update.Desired == nil || update.Actual == nil {
		return errors.New("update_pgbouncer action requires desired and actual PgBouncer state")
	}
	spec, err := pgBouncerContainerSpec(action)
	if err != nil {
		return err
	}
	pgBouncerDocker, ok := docker.(pgBouncerDockerClient)
	if !ok {
		return errors.New("docker client does not support PgBouncer updates")
	}
	if err := configurePgBouncerAuthentication(ctx, docker, action.ClusterID); err != nil {
		return err
	}
	if update.Actual.NetworkName != orcadocker.NetworkName(action.ClusterID) ||
		update.Actual.PublishedAddress != update.Desired.PgBouncer.PublishAddress ||
		update.Actual.PublishedPort != update.Desired.PgBouncer.PublishPort {
		if update.Actual.ContainerId != "" {
			if err := stopAndRemove(ctx, docker, update.Actual.ContainerId); err != nil {
				return err
			}
		}
		return createAndStart(ctx, docker, spec, nil)
	}

	changed, parseErr := pgbouncer.ChangedConfigKeys(update.Actual.Config, spec.Config.Content)
	method := pgbouncer.UpdateMethodRestart
	if parseErr == nil && update.Actual.Status == "running" {
		method = pgbouncer.ClassifyConfigUpdate(changed)
	}
	if err := pgBouncerDocker.WriteConfig(ctx, action.ClusterID, spec.Config); err != nil {
		return fmt.Errorf("write PgBouncer config: %w", err)
	}

	switch method {
	case pgbouncer.UpdateMethodReload:
		slog.Info("reloading PgBouncer configuration", "cluster_id", action.ClusterID)
		if err := pgbouncer.ReloadConfig(ctx, pgBouncerDocker, update.Actual.ContainerId); err != nil {
			rollbackErr := pgBouncerDocker.WriteConfig(ctx, action.ClusterID, &orcadocker.ConfigMount{
				RelativePath:  spec.Config.RelativePath,
				ContainerPath: spec.Config.ContainerPath,
				Content:       update.Actual.Config,
			})
			return errors.Join(err, rollbackErr)
		}
		return nil
	case pgbouncer.UpdateMethodRestart:
		slog.Info("restarting PgBouncer for configuration change", "cluster_id", action.ClusterID)
		if err := docker.StopContainer(ctx, update.Actual.ContainerId); err != nil {
			rollbackErr := pgBouncerDocker.WriteConfig(context.WithoutCancel(ctx), action.ClusterID, previousPgBouncerConfig(spec, update.Actual.Config))
			return errors.Join(err, rollbackErr)
		}
		if err := docker.StartContainer(ctx, update.Actual.ContainerId); err != nil {
			recoveryCtx := context.WithoutCancel(ctx)
			rollbackErr := pgBouncerDocker.WriteConfig(recoveryCtx, action.ClusterID, previousPgBouncerConfig(spec, update.Actual.Config))
			recoveryErr := docker.StartContainer(recoveryCtx, update.Actual.ContainerId)
			return errors.Join(err, rollbackErr, recoveryErr)
		}
		return nil
	default:
		return fmt.Errorf("unknown PgBouncer update method %q", method)
	}
}

func previousPgBouncerConfig(spec orcadocker.ContainerSpec, content string) *orcadocker.ConfigMount {
	return &orcadocker.ConfigMount{
		RelativePath:  spec.Config.RelativePath,
		ContainerPath: spec.Config.ContainerPath,
		Content:       content,
	}
}

func pgBouncerDesiredCluster(spec any) (*ClusterSpec, bool) {
	if cluster, ok := spec.(*ClusterSpec); ok {
		return cluster, true
	}
	if update, ok := spec.(*pgBouncerUpdateSpec); ok && update.Desired != nil {
		return update.Desired, true
	}
	return nil, false
}

func pgBouncerContainerSpec(action Action) (orcadocker.ContainerSpec, error) {
	if spec, ok := action.Spec.(orcadocker.ContainerSpec); ok {
		return spec, nil
	}
	cluster, ok := pgBouncerDesiredCluster(action.Spec)
	if !ok {
		return orcadocker.ContainerSpec{}, fmt.Errorf("%s action requires ClusterSpec", action.Type)
	}
	config, err := pgbouncer.GeneratePgBouncerConfig(cluster)
	if err != nil {
		return orcadocker.ContainerSpec{}, err
	}

	return orcadocker.ContainerSpec{
		ClusterID: action.ClusterID,
		Kind:      orcadocker.ContainerKindPgBouncer,
		Image:     "edoburu/pgbouncer:v1.25.2-p0",
		Ports: []orcadocker.PublishedPort{{
			ContainerPort: 6432,
			HostAddress:   cluster.PgBouncer.PublishAddress,
			HostPort:      uint16(cluster.PgBouncer.PublishPort),
		}},
		Config: &orcadocker.ConfigMount{
			RelativePath:  orcadocker.PgBouncerConfigRelativePath,
			ContainerPath: orcadocker.PgBouncerConfigContainerPath,
			Content:       config,
		},
	}, nil
}

type clusterCredentialDockerClient interface {
	EnsureClusterPassword(context.Context, string) (string, error)
	ExecContainer(context.Context, string, []string) (string, error)
	WriteConfig(context.Context, string, *orcadocker.ConfigMount) error
}

func createPgBouncer(ctx context.Context, docker DockerClient, action Action) error {
	if err := configurePgBouncerAuthentication(ctx, docker, action.ClusterID); err != nil {
		return err
	}
	spec, err := pgBouncerContainerSpec(action)
	return createAndStart(ctx, docker, spec, err)
}

func configurePgBouncerAuthentication(ctx context.Context, docker DockerClient, clusterID string) error {
	credentials, ok := docker.(clusterCredentialDockerClient)
	if !ok {
		return errors.New("docker client does not support cluster credentials")
	}
	password, err := credentials.EnsureClusterPassword(ctx, clusterID)
	if err != nil {
		return err
	}
	primary, err := orcadocker.ContainerName(orcadocker.ContainerSpec{ClusterID: clusterID, Kind: orcadocker.ContainerKindPrimary})
	if err != nil {
		return err
	}
	if err := synchronizePostgresPassword(ctx, credentials, primary, password); err != nil {
		return err
	}
	verifier, err := credentials.ExecContainer(ctx, primary, []string{
		"psql", "--username", "postgres", "--dbname", "postgres", "--tuples-only", "--no-align",
		"--command", "SELECT rolpassword FROM pg_authid WHERE rolname = 'postgres';",
	})
	if err != nil {
		return fmt.Errorf("read PostgreSQL SCRAM verifier: %w", err)
	}
	verifier = strings.TrimSpace(verifier)
	if !strings.HasPrefix(verifier, "SCRAM-SHA-256$") {
		return errors.New("PostgreSQL did not produce a SCRAM verifier")
	}
	if err := credentials.WriteConfig(ctx, clusterID, &orcadocker.ConfigMount{
		RelativePath: orcadocker.PgBouncerAuthRelativePath,
		Content:      fmt.Sprintf("\"postgres\" \"%s\"\n\"pgbouncer\" \"\"\n", verifier),
	}); err != nil {
		return fmt.Errorf("write PgBouncer auth file: %w", err)
	}
	return credentials.WriteConfig(ctx, clusterID, &orcadocker.ConfigMount{
		RelativePath: orcadocker.PgBouncerHbaRelativePath,
		Content: "local pgbouncer pgbouncer trust\n" +
			"host all all 0.0.0.0/0 scram-sha-256\n" +
			"host all all ::/0 scram-sha-256\n",
	})
}

func synchronizePostgresPassword(ctx context.Context, docker clusterCredentialDockerClient, containerID, password string) error {
	if password == "" || strings.ContainsAny(password, "'\\") {
		return errors.New("invalid generated PostgreSQL password")
	}
	_, err := docker.ExecContainer(ctx, containerID, []string{
		"psql", "--username", "postgres", "--dbname", "postgres", "--set", "ON_ERROR_STOP=1",
		"--command", "SET password_encryption = 'scram-sha-256'; ALTER ROLE postgres PASSWORD '" + password + "';",
	})
	if err != nil {
		return fmt.Errorf("synchronize PostgreSQL password: %w", err)
	}
	return nil
}

func pgBouncerContainerID(action Action) (string, error) {
	pgBouncer, ok := action.Spec.(*ActualPgBouncer)
	if !ok {
		return "", errors.New("delete_pgbouncer action requires ActualPgBouncer")
	}

	return pgBouncer.ContainerId, nil
}
