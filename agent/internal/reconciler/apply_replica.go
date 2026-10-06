package reconciler

import (
	"context"
	"errors"
	"fmt"
	orcadocker "github.com/swapnil404/orca/agent/internal/docker"
	"github.com/swapnil404/orca/agent/internal/postgres"
)

type replicaDockerClient interface {
	postgres.DockerClient
	postgres.ReplicaDockerClient
}

func createReplica(ctx context.Context, docker DockerClient, action Action, desired *DesiredState) error {
	replicaDocker, ok := docker.(replicaDockerClient)
	if !ok {
		return errors.New("docker client does not support replica provisioning")
	}
	cluster, err := desiredReplica(desired, action.ClusterID, action.ReplicaID)
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
	replicaID, err := postgres.CreateReplica(ctx, replicaDocker, postgres.ReplicaSpec{
		ClusterID:       cluster.Id,
		ReplicaID:       action.ReplicaID,
		PostgresVersion: cluster.Version,
		Params:          cluster.Params,
		Primary: postgres.PrimaryConnectionInfo{
			Host: primary,
		},
	})
	if err != nil {
		return err
	}
	if cluster.PgHba != nil {
		if err := postgres.ApplyReplicaHBA(ctx, replicaDocker, cluster, replicaID); err != nil {
			return fmt.Errorf("apply replica pg_hba.conf: %w", err)
		}
	}
	return markReplicaParamsApplied(ctx, replicaDocker, replicaID, cluster, action.ReplicaID)
}

func desiredReplica(desired *DesiredState, clusterID, replicaID string) (*ClusterSpec, error) {
	for _, cluster := range desired.Clusters {
		if cluster == nil || cluster.Id != clusterID {
			continue
		}
		for _, replica := range cluster.Replicas {
			if replica != nil && replica.Id == replicaID {
				return cluster, nil
			}
		}
		return nil, fmt.Errorf("replica %q is not desired for cluster %q", replicaID, clusterID)
	}
	return nil, fmt.Errorf("cluster %q is not desired", clusterID)
}

func markReplicaParamsApplied(ctx context.Context, docker replicaDockerClient, containerID string, cluster *ClusterSpec, replicaID string) error {
	if err := postgres.WaitForConfigApplied(ctx, docker, containerID, len(cluster.Params)); err != nil {
		return err
	}
	identity, err := postgres.DeriveReplicaIdentity(cluster.Id, replicaID)
	if err != nil {
		return err
	}
	configDocker, ok := any(docker).(interface {
		WriteConfig(context.Context, string, *orcadocker.ConfigMount) error
	})
	if !ok {
		return errors.New("docker client does not support replica parameter state")
	}
	return writeAppliedParams(ctx, configDocker, cluster.Id, orcadocker.PostgresReplicaAppliedConfigRelativePath(replicaID), identity.DataPath, cluster.Params)
}

func replicaContainerID(action Action) (string, error) {
	replica, ok := action.Spec.(*ActualReplica)
	if deleteSpec, deleteOK := action.Spec.(*replicaDeleteSpec); deleteOK {
		replica, ok = deleteSpec.Actual, deleteSpec.Actual != nil
	}
	if !ok || replica == nil {
		return "", errors.New("delete_replica action requires ActualReplica")
	}

	return replica.ContainerId, nil
}
