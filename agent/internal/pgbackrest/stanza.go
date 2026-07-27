package pgbackrest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	orcadocker "github.com/swapnil404/orca/agent/internal/docker"
)

type infoStanza struct {
	Backup []struct {
		Error     bool `json:"error"`
		Timestamp struct {
			Stop int64 `json:"stop"`
		} `json:"timestamp"`
	} `json:"backup"`
}

// Executor runs pgBackRest commands against the PostgreSQL primary.
type Executor interface {
	ExecContainer(ctx context.Context, containerID string, command []string) (string, error)
}

// InitializeStanza creates the cluster stanza if it does not already exist.
func InitializeStanza(ctx context.Context, executor Executor, desired *ClusterDesiredState) error {
	if desired == nil {
		return errors.New("desired cluster is nil")
	}
	if executor == nil {
		return errors.New("executor is nil")
	}
	if _, err := GeneratePgBackRestConfig(desired); err != nil {
		return err
	}

	primary, err := primaryContainerName(desired.Id)
	if err != nil {
		return err
	}

	if _, err := executor.ExecContainer(ctx, primary, pgBackRestCommand(desired.Id, "info")); err == nil {
		return nil
	}
	if _, err := executor.ExecContainer(ctx, primary, pgBackRestCommand(desired.Id, "stanza-create")); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "already exists") {
			return nil
		}
		return fmt.Errorf("create pgBackRest stanza %q: %w", desired.Id, err)
	}
	return nil
}

// LastSuccessfulBackup reads the latest completed backup from pgBackRest repository metadata.
func LastSuccessfulBackup(ctx context.Context, executor Executor, containerID, clusterID string) (int64, bool, error) {
	if executor == nil || containerID == "" || clusterID == "" {
		return 0, false, errors.New("pgBackRest observation requires executor, container, and cluster")
	}
	command := []string{"gosu", postgresUser, "pgbackrest", "--config=" + clusterConfigPath(clusterID), "--stanza=" + clusterID, "--output=json", "info"}
	output, err := executor.ExecContainer(ctx, containerID, command)
	if err != nil {
		return 0, false, fmt.Errorf("read pgBackRest info: %w", err)
	}
	var stanzas []infoStanza
	if err := json.Unmarshal([]byte(output), &stanzas); err != nil {
		return 0, false, fmt.Errorf("decode pgBackRest info: %w", err)
	}
	var latest int64
	for _, stanza := range stanzas {
		for _, backup := range stanza.Backup {
			if !backup.Error && backup.Timestamp.Stop > latest {
				latest = backup.Timestamp.Stop
			}
		}
	}
	return latest, latest > 0, nil
}

func primaryContainerName(clusterID string) (string, error) {
	primary, err := orcadocker.ContainerName(orcadocker.ContainerSpec{
		ClusterID: clusterID,
		Kind:      orcadocker.ContainerKindPrimary,
	})
	if err != nil {
		return "", fmt.Errorf("resolve primary: %w", err)
	}
	return primary, nil
}

func pgBackRestCommand(stanza, operation string) []string {
	return []string{"gosu", postgresUser, "pgbackrest", "--config=" + clusterConfigPath(stanza), "--stanza=" + stanza, operation}
}
