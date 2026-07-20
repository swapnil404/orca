package reconciler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	orcadocker "github.com/betterorca/betterorca/agent/internal/docker"
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
func Apply(ctx context.Context, docker DockerClient, actions []Action) []ApplyResult {
	results := make([]ApplyResult, 0, len(actions))
	for _, action := range actions {
		results = append(results, ApplyResult{
			Action: action,
			Err:    applyAction(ctx, docker, action),
		})
	}

	return results
}

func applyAction(ctx context.Context, docker DockerClient, action Action) error {
	if docker == nil {
		return errors.New("docker client is nil")
	}

	switch action.Type {
	case ActionCreatePrimary, ActionUpdatePrimary:
		spec, err := primaryContainerSpec(action)
		return createAndStart(ctx, docker, spec, err)
	case ActionCreateReplica:
		spec, err := replicaContainerSpec(action)
		return createAndStart(ctx, docker, spec, err)
	case ActionCreatePgBouncer:
		spec, err := pgBouncerContainerSpec(action)
		return createAndStart(ctx, docker, spec, err)
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
	if _, ok := action.Spec.(*PgBouncerSpec); !ok {
		return orcadocker.ContainerSpec{}, errors.New("create_pgbouncer action requires PgBouncerSpec")
	}

	return orcadocker.ContainerSpec{
		ClusterID: action.ClusterID,
		Kind:      orcadocker.ContainerKindPgBouncer,
		Image:     "pgbouncer:latest",
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
