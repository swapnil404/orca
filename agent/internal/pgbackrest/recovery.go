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

// RestoreToTime restores a primary to target, pauses at the target for a
// consistency check, promotes it, and verifies that it is read-write.
func RestoreToTime(ctx context.Context, executor RecoveryExecutor, desired *ClusterDesiredState, target time.Time) error {
	if executor == nil {
		return errors.New("executor is nil")
	}
	if desired == nil {
		return errors.New("desired cluster is nil")
	}
	if target.IsZero() {
		return errors.New("recovery target time is required")
	}
	config, err := GeneratePgBackRestConfig(*desired)
	if err != nil {
		return err
	}
	if err := validateRecoveryRepository(desired); err != nil {
		return err
	}
	primary, err := primaryContainerName(desired.Id)
	if err != nil {
		return err
	}

	if err := executor.StopContainer(ctx, primary); err != nil {
		return fmt.Errorf("stop primary for point-in-time recovery: %w", err)
	}

	restoreStarted := false
	restoreID := ""
	recoverStartup := func(cause error) error {
		var cleanupErr error
		if restoreID != "" {
			cleanupErr = removeRecoveryContainer(ctx, executor, restoreID)
		}
		if !restoreStarted {
			cleanupErr = errors.Join(cleanupErr, executor.StartContainer(ctx, primary))
		}
		return errors.Join(cause, cleanupErr)
	}

	restoreID, err = executor.CreateContainer(ctx, recoveryContainerSpec(desired, config))
	if err != nil {
		return recoverStartup(fmt.Errorf("create pgBackRest recovery container: %w", err))
	}
	if err := executor.StartContainer(ctx, restoreID); err != nil {
		return recoverStartup(fmt.Errorf("start pgBackRest recovery container: %w", err))
	}
	restoreStarted = true
	if _, err := executor.ExecContainer(ctx, restoreID, restoreCommand(desired.Id, target)); err != nil {
		return recoverStartup(fmt.Errorf("restore stanza %q to %s: %w", desired.Id, target.Format(time.RFC3339Nano), err))
	}
	if err := removeRecoveryContainer(ctx, executor, restoreID); err != nil {
		return fmt.Errorf("remove pgBackRest recovery container: %w", err)
	}
	restoreID = ""

	if err := executor.StartContainer(ctx, primary); err != nil {
		return fmt.Errorf("start restored primary in recovery: %w", err)
	}
	if err := waitForRecoveryTarget(ctx, executor, primary); err != nil {
		return err
	}
	if _, err := executor.ExecContainer(ctx, primary, psqlCommand("SELECT pg_wal_replay_resume()")); err != nil {
		return fmt.Errorf("resume WAL replay at recovery target: %w", err)
	}
	if err := waitForReadWrite(ctx, executor, primary); err != nil {
		return err
	}
	return nil
}

func validateRecoveryRepository(desired *ClusterDesiredState) error {
	volumePath := filepath.Clean(orcadocker.VolumeMountPath(desired.Id))
	repositoryPath := filepath.Clean(desired.PgBackRest.RepoPath)
	relative, err := filepath.Rel(volumePath, repositoryPath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("pgBackRest repository %q must be within shared cluster volume %q for recovery", repositoryPath, volumePath)
	}
	return nil
}

func recoveryContainerSpec(desired *ClusterDesiredState, config string) orcadocker.ContainerSpec {
	image := "postgres:latest"
	if desired.Version != "" {
		image = "postgres:" + desired.Version
	}
	return orcadocker.ContainerSpec{
		ClusterID: desired.Id,
		Kind:      orcadocker.ContainerKindPgBackRest,
		Image:     image,
		Env:       []string{"PGDATA=" + orcadocker.VolumeMountPath(desired.Id) + "/primary"},
		Command:   []string{"sleep", "infinity"},
		UseVolume: true,
		Config: &orcadocker.ConfigMount{
			RelativePath:  configRelativePath,
			ContainerPath: configPath,
			Content:       config,
		},
	}
}

func restoreCommand(clusterID string, target time.Time) []string {
	return []string{
		"sh", "-c",
		`install -d -m 0700 -o postgres -g postgres "$1" && shift && exec gosu postgres pgbackrest "$@"`,
		"sh", orcadocker.VolumeMountPath(clusterID) + "/primary",
		"--stanza=" + clusterID,
		"--delta",
		"--type=time",
		"--target=" + target.Format("2006-01-02 15:04:05.000000-07:00"),
		"--target-action=pause",
		"restore",
	}
}

func removeRecoveryContainer(ctx context.Context, executor RecoveryExecutor, containerID string) error {
	stopErr := executor.StopContainer(ctx, containerID)
	removeErr := executor.RemoveContainer(ctx, containerID)
	return errors.Join(stopErr, removeErr)
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
