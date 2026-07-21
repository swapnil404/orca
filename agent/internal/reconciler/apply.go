package reconciler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	orcadocker "github.com/swapnil404/orca/agent/internal/docker"
	"github.com/swapnil404/orca/agent/internal/pgbouncer"
	"github.com/swapnil404/orca/agent/internal/postgres"
)

// DockerClient is the Docker wrapper interface used by Apply.
type DockerClient = orcadocker.DockerClient

// ApplyResult reports the outcome of executing one action.
type ApplyResult struct {
	Action Action
	Err    error
}

// MarshalJSON encodes an apply error as a readable string or null.
func (r ApplyResult) MarshalJSON() ([]byte, error) {
	var applyError *string
	if r.Err != nil {
		message := r.Err.Error()
		applyError = &message
	}

	return json.Marshal(struct {
		Action Action
		Err    *string
	}{
		Action: r.Action,
		Err:    applyError,
	})
}

// Apply executes every action against Docker and reports each result.
func Apply(ctx context.Context, docker DockerClient, actions []Action, desiredStates ...DesiredState) []ApplyResult {
	var desired DesiredState
	if len(desiredStates) > 0 {
		desired = desiredStates[0]
	}
	results := make([]ApplyResult, 0, len(actions))
	for _, action := range actions {
		results = append(results, ApplyResult{
			Action: action,
			Err:    applyAction(ctx, docker, action, desired),
		})
	}

	return results
}

func applyAction(ctx context.Context, docker DockerClient, action Action, desired DesiredState) error {
	if docker == nil {
		return errors.New("docker client is nil")
	}

	switch action.Type {
	case ActionCreatePrimary, ActionUpdatePrimary:
		spec, err := primaryContainerSpec(action)
		return createAndStart(ctx, docker, spec, err)
	case ActionCreateReplica:
		return createReplica(ctx, docker, action, desired)
	case ActionCreatePgBouncer:
		spec, err := pgBouncerContainerSpec(action)
		return createAndStart(ctx, docker, spec, err)
	case ActionUpdatePgBouncer:
		return updatePgBouncer(ctx, docker, action)
	case ActionDeletePrimary:
		cluster, ok := action.Spec.(*ActualCluster)
		if !ok {
			return errors.New("delete_primary action requires ActualCluster")
		}
		containerID, err := primaryContainerID(action)
		if err != nil {
			return err
		}
		if err := stopAndRemove(ctx, docker, containerID); err != nil {
			return err
		}
		return docker.RemoveVolume(ctx, orcadocker.VolumeName(cluster.Id))
	case ActionDeleteReplica:
		containerID, err := replicaContainerID(action)
		if err != nil {
			return err
		}
		return stopAndRemove(ctx, docker, containerID)
	case ActionDeletePgBouncer:
		containerID, err := pgBouncerContainerID(action)
		if err != nil {
			return err
		}
		return stopAndRemove(ctx, docker, containerID)
	default:
		return fmt.Errorf("unknown action type %q", action.Type)
	}
}

type replicaDockerClient interface {
	postgres.DockerClient
	postgres.ReplicaDockerClient
	ContainerNetworkAddresses(context.Context, string) ([]string, error)
}

type pgBouncerDockerClient interface {
	DockerClient
	pgbouncer.ConsoleExecutor
	WriteConfig(ctx context.Context, clusterID string, config *orcadocker.ConfigMount) error
}

func createReplica(ctx context.Context, docker DockerClient, action Action, desired DesiredState) error {
	replicaDocker, ok := docker.(replicaDockerClient)
	if !ok {
		return errors.New("docker client does not support replica provisioning")
	}
	cluster, index, err := desiredReplica(desired, action.ClusterID, action.ReplicaID)
	if err != nil {
		return err
	}
	if err := postgres.ConfigurePrimaryReplication(ctx, replicaDocker, cluster); err != nil {
		return fmt.Errorf("configure primary replication: %w", err)
	}
	primary, err := orcadocker.ContainerName(orcadocker.ContainerSpec{
		ClusterID: action.ClusterID,
		Kind:      orcadocker.ContainerKindPrimary,
	})
	if err != nil {
		return err
	}
	addresses, err := replicaDocker.ContainerNetworkAddresses(ctx, primary)
	if err != nil {
		return fmt.Errorf("inspect primary address: %w", err)
	}
	if len(addresses) == 0 {
		return errors.New("primary container has no network address")
	}
	_, err = postgres.CreateReplica(ctx, replicaDocker, postgres.ReplicaSpec{
		ClusterID:       cluster.Id,
		ReplicaID:       action.ReplicaID,
		Index:           index,
		PostgresVersion: cluster.Version,
		Primary: postgres.PrimaryConnectionInfo{
			Host: addresses[0],
		},
	})
	return err
}

func desiredReplica(desired DesiredState, clusterID, replicaID string) (*ClusterSpec, int, error) {
	for _, cluster := range desired.Clusters {
		if cluster == nil || cluster.Id != clusterID {
			continue
		}
		for index, replica := range cluster.Replicas {
			if replica != nil && replica.Id == replicaID {
				return cluster, index + 1, nil
			}
		}
		return nil, 0, fmt.Errorf("replica %q is not desired for cluster %q", replicaID, clusterID)
	}
	return nil, 0, fmt.Errorf("cluster %q is not desired", clusterID)
}

func createAndStart(ctx context.Context, docker DockerClient, spec orcadocker.ContainerSpec, specErr error) error {
	if specErr != nil {
		return specErr
	}

	containerID, err := docker.CreateContainer(ctx, spec)
	if err != nil {
		return err
	}

	return docker.StartContainer(ctx, containerID)
}

func stopAndRemove(ctx context.Context, docker DockerClient, containerID string) error {
	if containerID == "" {
		return errors.New("container ID is required")
	}
	if err := docker.StopContainer(ctx, containerID); err != nil {
		return err
	}

	return docker.RemoveContainer(ctx, containerID)
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
			rollbackErr := pgBouncerDocker.WriteConfig(ctx, action.ClusterID, &orcadocker.ConfigMount{
				RelativePath:  spec.Config.RelativePath,
				ContainerPath: spec.Config.ContainerPath,
				Content:       update.Actual.Config,
			})
			return errors.Join(err, rollbackErr)
		}
		return docker.StartContainer(ctx, update.Actual.ContainerId)
	default:
		return fmt.Errorf("unknown PgBouncer update method %q", method)
	}
}

func primaryContainerSpec(action Action) (orcadocker.ContainerSpec, error) {
	if spec, ok := action.Spec.(orcadocker.ContainerSpec); ok {
		return spec, nil
	}

	cluster, ok := action.Spec.(*ClusterSpec)
	if !ok {
		return orcadocker.ContainerSpec{}, fmt.Errorf("%s action requires ClusterSpec", action.Type)
	}

	return orcadocker.ContainerSpec{
		ClusterID: cluster.Id,
		Kind:      orcadocker.ContainerKindPrimary,
		Image:     postgresImage(cluster.Version),
		Env: []string{
			"POSTGRES_HOST_AUTH_METHOD=trust",
			"PGDATA=" + orcadocker.VolumeMountPath(cluster.Id) + "/primary",
		},
		UseVolume: true,
	}, nil
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

func replicaContainerSpec(action Action) (orcadocker.ContainerSpec, error) {
	if spec, ok := action.Spec.(orcadocker.ContainerSpec); ok {
		return spec, nil
	}

	replica, ok := action.Spec.(*ReplicaSpec)
	if !ok {
		return orcadocker.ContainerSpec{}, errors.New("create_replica action requires ReplicaSpec")
	}

	return orcadocker.ContainerSpec{
		ClusterID: action.ClusterID,
		Kind:      orcadocker.ContainerKindReplica,
		ReplicaID: replica.Id,
		Image:     postgresImage(""),
		Env: []string{
			"POSTGRES_HOST_AUTH_METHOD=trust",
			"PGDATA=" + orcadocker.VolumeMountPath(action.ClusterID) + "/replicas/" + replica.Id,
		},
		UseVolume: true,
	}, nil
}

func pgBouncerContainerSpec(action Action) (orcadocker.ContainerSpec, error) {
	if spec, ok := action.Spec.(orcadocker.ContainerSpec); ok {
		return spec, nil
	}
	cluster, ok := pgBouncerDesiredCluster(action.Spec)
	if !ok {
		return orcadocker.ContainerSpec{}, fmt.Errorf("%s action requires ClusterSpec", action.Type)
	}
	config, err := pgbouncer.GeneratePgBouncerConfig(*cluster)
	if err != nil {
		return orcadocker.ContainerSpec{}, err
	}

	return orcadocker.ContainerSpec{
		ClusterID: action.ClusterID,
		Kind:      orcadocker.ContainerKindPgBouncer,
		Image:     "pgbouncer:latest",
		Config: &orcadocker.ConfigMount{
			RelativePath:  orcadocker.PgBouncerConfigRelativePath,
			ContainerPath: orcadocker.PgBouncerConfigContainerPath,
			Content:       config,
		},
	}, nil
}

func primaryContainerID(action Action) (string, error) {
	cluster, ok := action.Spec.(*ActualCluster)
	if !ok {
		return "", errors.New("delete_primary action requires ActualCluster")
	}

	return cluster.ContainerId, nil
}

func replicaContainerID(action Action) (string, error) {
	replica, ok := action.Spec.(*ActualReplica)
	if !ok {
		return "", errors.New("delete_replica action requires ActualReplica")
	}

	return replica.ContainerId, nil
}

func pgBouncerContainerID(action Action) (string, error) {
	pgBouncer, ok := action.Spec.(*ActualPgBouncer)
	if !ok {
		return "", errors.New("delete_pgbouncer action requires ActualPgBouncer")
	}

	return pgBouncer.ContainerId, nil
}

func postgresImage(version string) string {
	if version == "" {
		return "postgres:latest"
	}

	return "postgres:" + version
}
