package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	orcadocker "github.com/swapnil404/orca/agent/internal/docker"
	orcatypes "github.com/swapnil404/orca/pkg/types"
)

const (
	postgresUser         = "postgres"
	hbaChanged           = "changed"
	primaryReadyTimeout  = 30 * time.Second
	primaryReadyInterval = 250 * time.Millisecond
)

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
	if err := waitForPrimary(ctx, docker, primary); err != nil {
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
	}

	cidrs, err := docker.ContainerNetworkCIDRs(ctx, primary)
	if err != nil {
		return fmt.Errorf("inspect primary network: %w", err)
	}
	if len(cidrs) == 0 {
		return errors.New("primary container has no network CIDR")
	}

	hbaWasChanged := false
	for _, cidr := range cidrs {
		line := fmt.Sprintf("host replication all %s trust", cidr)
		output, err := docker.ExecContainer(ctx, primary, ensureHBALineCommand(line))
		if err != nil {
			return fmt.Errorf("allow replication from network %q: %w", cidr, err)
		}
		hbaWasChanged = hbaWasChanged || strings.TrimSpace(output) == hbaChanged
	}

	for index := range desired.Replicas {
		slot := fmt.Sprintf("replica_%d", index+1)
		query := fmt.Sprintf("SELECT pg_create_physical_replication_slot('%s') WHERE NOT EXISTS (SELECT 1 FROM pg_replication_slots WHERE slot_name = '%s')", slot, slot)
		if _, err := docker.ExecContainer(ctx, primary, psqlCommand(query)); err != nil {
			return fmt.Errorf("ensure replication slot %q: %w", slot, err)
		}
	}

	if hbaWasChanged {
		if _, err := docker.ExecContainer(ctx, primary, psqlCommand("SELECT pg_reload_conf()")); err != nil {
			return fmt.Errorf("reload PostgreSQL config: %w", err)
		}
	}

	return nil
}

func waitForPrimary(ctx context.Context, docker DockerClient, primary string) error {
	ctx, cancel := context.WithTimeout(ctx, primaryReadyTimeout)
	defer cancel()

	var lastErr error
	for {
		if _, err := docker.ExecContainer(ctx, primary, psqlCommand("SELECT 1")); err == nil {
			return nil
		} else {
			lastErr = err
		}

		timer := time.NewTimer(primaryReadyInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("wait for primary readiness: %w", errors.Join(lastErr, ctx.Err()))
		case <-timer.C:
		}
	}
}

func psqlCommand(query string) []string {
	return []string{"psql", "-U", postgresUser, "-Atqc", query}
}

func ensureHBALineCommand(line string) []string {
	return []string{
		"sh", "-c",
		`hba_file="$(psql -U postgres -Atqc 'SHOW hba_file')"; if grep -Fqx -- "$1" "$hba_file"; then printf unchanged; else printf '%s\n' "$1" >> "$hba_file"; printf changed; fi`,
		"sh", line,
	}
}
