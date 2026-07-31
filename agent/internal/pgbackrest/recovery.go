package pgbackrest

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	orcadocker "github.com/swapnil404/orca/agent/internal/docker"
)

const (
	recoveryReadyTimeout  = 2 * time.Minute
	recoveryReadyInterval = 250 * time.Millisecond
	configRelativePath    = "pgbackrest/pgbackrest.conf"
)

// RecoveryExecutor provides the Docker operations needed for point-in-time recovery.
type RecoveryExecutor interface {
	Executor
	CreateContainer(ctx context.Context, spec orcadocker.ContainerSpec) (string, error)
	StartContainer(ctx context.Context, containerID string) error
	StopContainer(ctx context.Context, containerID string) error
	RemoveContainer(ctx context.Context, containerID string) error
}

// RestoreToTime rejects the legacy unjournaled restore path. RestoreManager is
// required so destructive recovery can be resumed or rolled back after a crash.
// Deprecated: submit a RestoreOperation through the reconciliation runner.
func RestoreToTime(ctx context.Context, executor RecoveryExecutor, desired *ClusterDesiredState, target time.Time) error {
	return errors.New("unjournaled restore is disabled; use RestoreManager through the reconciliation runner")
}

func validateRecoveryRepository(desired *ClusterDesiredState) error {
	volumePath := filepath.Clean(orcadocker.VolumeMountPath(desired.Id))
	repositoryPath := filepath.Clean(desired.PgBackRest.RepoPath)
	relative, err := filepath.Rel(volumePath, repositoryPath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("pgBackRest repository %q must be within shared cluster volume %q for recovery", repositoryPath, volumePath)
	}
	first, _, _ := strings.Cut(relative, string(filepath.Separator))
	if relative == "." || first == "primary" || first == "replicas" || first == "pgbackrest" ||
		strings.HasPrefix(first, "restore-original-") || strings.HasPrefix(first, ".orca-restore-") {
		return fmt.Errorf("pgBackRest repository %q overlaps Orca-managed data inside %q", repositoryPath, volumePath)
	}
	return nil
}

func waitForRecoveryTarget(ctx context.Context, executor Executor, primary string) error {
	return waitForRecoveryState(ctx, executor, primary,
		"SELECT pg_is_in_recovery()::text || '|' || pg_is_wal_replay_paused()::text",
		func(output string) bool {
			return strings.TrimSpace(output) == "true|true" || strings.TrimSpace(output) == "t|t"
		},
		"wait for restored primary to reach a consistent paused recovery target",
	)
}

func waitForReadWrite(ctx context.Context, executor Executor, primary string) error {
	return waitForRecoveryState(ctx, executor, primary,
		"SELECT pg_is_in_recovery()::text || '|' || current_setting('transaction_read_only')",
		func(output string) bool {
			return strings.TrimSpace(output) == "false|off" || strings.TrimSpace(output) == "f|off"
		},
		"wait for restored primary to become read-write",
	)
}

func waitForRecoveryState(ctx context.Context, executor Executor, primary, query string, ready func(string) bool, operation string) error {
	waitCtx, cancel := context.WithTimeout(ctx, recoveryReadyTimeout)
	defer cancel()

	var lastErr error
	for {
		output, err := executor.ExecContainer(waitCtx, primary, psqlCommand(query))
		if err == nil && ready(output) {
			return nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("unexpected state %q", strings.TrimSpace(output))
		}

		timer := time.NewTimer(recoveryReadyInterval)
		select {
		case <-waitCtx.Done():
			timer.Stop()
			return fmt.Errorf("%s: %w", operation, errors.Join(lastErr, waitCtx.Err()))
		case <-timer.C:
		}
	}
}
