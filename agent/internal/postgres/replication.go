package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	orcadocker "github.com/swapnil404/orca/agent/internal/docker"
	orcatypes "github.com/swapnil404/orca/pkg/types"
)

const postgresUser = "postgres"

// ClusterDesiredState describes the desired state of one PostgreSQL cluster.
type ClusterDesiredState = orcatypes.ClusterSpec

// DockerClient is the Docker functionality required to configure PostgreSQL.
type DockerClient interface {
	ExecContainer(ctx context.Context, containerID string, command []string) (string, error)
	RestartContainer(ctx context.Context, containerID string) error
	ContainerNetworkCIDRs(ctx context.Context, containerID string) ([]string, error)
}

// ConfigurePrimaryReplication configures a running primary for its desired replicas.
func ConfigurePrimaryReplication(ctx context.Context, docker DockerClient, desired *ClusterDesiredState) error {
	if desired == nil || len(desired.Replicas) == 0 {
		return nil
	}
	if docker == nil {
		return errors.New("docker client is nil")
	}

	primary, err := orcadocker.ContainerName(orcadocker.ContainerSpec{
		ClusterID: desired.Id,
		Kind:      orcadocker.ContainerKindPrimary,
	})
	if err != nil {
		return err
	}
	if err := WaitForPrimaryReady(ctx, docker, primary); err != nil {
		return err
	}

	walLevel, err := docker.ExecContainer(ctx, primary, psqlCommand("SHOW wal_level"))
	if err != nil {
		return fmt.Errorf("show wal_level: %w", err)
	}
	if strings.TrimSpace(walLevel) != "replica" {
		if _, err := docker.ExecContainer(ctx, primary, psqlCommand("ALTER SYSTEM SET wal_level = 'replica'")); err != nil {
			return fmt.Errorf("set wal_level: %w", err)
		}
		if err := docker.RestartContainer(ctx, primary); err != nil {
			return fmt.Errorf("restart primary after changing wal_level: %w", err)
		}
		if err := WaitForPrimaryReady(ctx, docker, primary); err != nil {
			return fmt.Errorf("wait for primary after changing wal_level: %w", err)
		}
	}

	for _, replica := range desired.Replicas {
		identity, err := DeriveReplicaIdentity(desired.Id, replica.Id)
		if err != nil {
			return err
		}
		slot := identity.SlotName
		query := fmt.Sprintf("SELECT pg_create_physical_replication_slot('%s') WHERE NOT EXISTS (SELECT 1 FROM pg_replication_slots WHERE slot_name = '%s')", slot, slot)
		if _, err := docker.ExecContainer(ctx, primary, psqlCommand(query)); err != nil {
			return fmt.Errorf("ensure replication slot %q: %w", slot, err)
		}
	}

	return nil
}

// DeleteReplica stops a replica, cleans up its data and replication slot, then removes its container.
func DeleteReplica(ctx context.Context, docker ReplicaDockerClient, clusterID, replicaID, containerID string) error {
	if docker == nil {
		return errors.New("docker client is nil")
	}
	identity, err := DeriveReplicaIdentity(clusterID, replicaID)
	if err != nil {
		return err
	}
	if err := docker.StopContainer(ctx, containerID); err != nil {
		return err
	}
	primary, err := orcadocker.ContainerName(orcadocker.ContainerSpec{
		ClusterID: clusterID,
		Kind:      orcadocker.ContainerKindPrimary,
	})
	if err != nil {
		return err
	}
	_, dataErr := docker.ExecContainer(ctx, primary, []string{"rm", "-rf", "--", identity.DataPath})
	slotErr := dropReplicationSlot(ctx, docker, primary, identity.SlotName)
	if err := errors.Join(dataErr, slotErr); err != nil {
		return err
	}
	return docker.RemoveContainer(ctx, containerID)
}

func dropReplicationSlot(ctx context.Context, docker ReplicaDockerClient, primary, slotName string) error {
	dropSlot := fmt.Sprintf("SELECT pg_drop_replication_slot('%s') WHERE EXISTS (SELECT 1 FROM pg_replication_slots WHERE slot_name = '%s')", slotName, slotName)
	_, err := docker.ExecContainer(ctx, primary, psqlCommand(dropSlot))
	return err
}

func psqlCommand(query string) []string {
	return []string{"psql", "-U", postgresUser, "-Atqc", query}
}
