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
	Name   string `json:"name"`
	Status struct {
		Code int `json:"code"`
	} `json:"status"`
	Backup []struct {
		Error bool `json:"error"`
		Info  struct {
			Repository struct {
				Size uint64 `json:"size"`
			} `json:"repository"`
		} `json:"info"`
		Timestamp struct {
			Stop int64 `json:"stop"`
		} `json:"timestamp"`
	} `json:"backup"`
}

// BackupObservation describes the latest pgBackRest attempt and successful backup.
type BackupObservation struct {
	LastSuccessUnixSeconds int64
	SizeBytes              uint64
	Status                 string
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

	command := []string{"gosu", postgresUser, "pgbackrest", "--config=" + clusterConfigPath(desired.Id), "--stanza=" + desired.Id, "--output=json", "info"}
	if output, err := executor.ExecContainer(ctx, primary, command); err == nil {
		var stanzas []infoStanza
		if json.Unmarshal([]byte(output), &stanzas) == nil {
			for _, stanza := range stanzas {
				if stanza.Name == desired.Id && stanza.Status.Code == 0 {
					return nil
				}
			}
		}
	}
	if _, err := executor.ExecContainer(ctx, primary, pgBackRestCommand(desired.Id, "stanza-create")); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "already exists") {
			return nil
		}
		return fmt.Errorf("create pgBackRest stanza %q: %w", desired.Id, err)
	}
	return nil
}

// ObserveBackups reads the latest backup attempt from pgBackRest repository metadata.
func ObserveBackups(ctx context.Context, executor Executor, containerID, clusterID string) (BackupObservation, error) {
	if executor == nil || containerID == "" || clusterID == "" {
		return BackupObservation{}, errors.New("pgBackRest observation requires executor, container, and cluster")
	}
	command := []string{"gosu", postgresUser, "pgbackrest", "--config=" + clusterConfigPath(clusterID), "--stanza=" + clusterID, "--output=json", "info"}
	output, err := executor.ExecContainer(ctx, containerID, command)
	if err != nil {
		return BackupObservation{}, fmt.Errorf("read pgBackRest info: %w", err)
	}
	var stanzas []infoStanza
	if err := json.Unmarshal([]byte(output), &stanzas); err != nil {
		return BackupObservation{}, fmt.Errorf("decode pgBackRest info: %w", err)
	}
	observation := BackupObservation{Status: "pending"}
	var latestAttempt int64
	for _, stanza := range stanzas {
		for _, backup := range stanza.Backup {
			if backup.Timestamp.Stop > latestAttempt {
				latestAttempt = backup.Timestamp.Stop
				if backup.Error {
					observation.Status = "failed"
				} else {
					observation.Status = "succeeded"
				}
			}
			if !backup.Error && backup.Timestamp.Stop > observation.LastSuccessUnixSeconds {
				observation.LastSuccessUnixSeconds = backup.Timestamp.Stop
				observation.SizeBytes = backup.Info.Repository.Size
			}
		}
	}
	return observation, nil
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
