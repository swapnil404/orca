package pgbackrest

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const postgresUser = "postgres"

// PrimaryExecutor runs commands and restarts the PostgreSQL primary.
type PrimaryExecutor interface {
	Executor
	RestartContainer(ctx context.Context, containerID string) error
}

// ConfigureWALArchiving enables pgBackRest WAL archiving on the primary.
func ConfigureWALArchiving(ctx context.Context, executor PrimaryExecutor, desired *ClusterDesiredState) error {
	if desired == nil {
		return errors.New("desired cluster is nil")
	}
	if executor == nil {
		return errors.New("executor is nil")
	}
	if _, err := GeneratePgBackRestConfig(*desired); err != nil {
		return err
	}

	primary, err := primaryContainerName(desired.Id)
	if err != nil {
		return err
	}
	archiveCommand := fmt.Sprintf("pgbackrest --stanza=%s archive-push %%p", desired.Id)

	mode, err := executor.ExecContainer(ctx, primary, psqlCommand("SHOW archive_mode"))
	if err != nil {
		return fmt.Errorf("show archive_mode: %w", err)
	}
	command, err := executor.ExecContainer(ctx, primary, psqlCommand("SHOW archive_command"))
	if err != nil {
		return fmt.Errorf("show archive_command: %w", err)
	}

	modeChanged := strings.TrimSpace(mode) != "on"
	commandChanged := strings.TrimSpace(command) != archiveCommand
	if modeChanged {
		if _, err := executor.ExecContainer(ctx, primary, psqlCommand("ALTER SYSTEM SET archive_mode = 'on'")); err != nil {
			return fmt.Errorf("enable archive_mode: %w", err)
		}
	}
	if commandChanged {
		query := fmt.Sprintf("ALTER SYSTEM SET archive_command = %s", quotePostgresConfig(archiveCommand))
		if _, err := executor.ExecContainer(ctx, primary, psqlCommand(query)); err != nil {
			return fmt.Errorf("set archive_command: %w", err)
		}
	}

	// PostgreSQL documents archive_mode as server-start-only. archive_command
	// alone is reloadable, so avoid a full restart when only it changed.
	if modeChanged {
		if err := executor.RestartContainer(ctx, primary); err != nil {
			return fmt.Errorf("restart primary after enabling archive_mode: %w", err)
		}
	} else if commandChanged {
		if _, err := executor.ExecContainer(ctx, primary, psqlCommand("SELECT pg_reload_conf()")); err != nil {
			return fmt.Errorf("reload archive_command: %w", err)
		}
	}

	return nil
}

// DisableWALArchiving removes pgBackRest archiving from a retained primary.
func DisableWALArchiving(ctx context.Context, executor PrimaryExecutor, clusterID string) error {
	if executor == nil {
		return errors.New("executor is nil")
	}
	if err := validateClusterID(clusterID); err != nil {
		return err
	}
	primary, err := primaryContainerName(clusterID)
	if err != nil {
		return err
	}
	mode, err := executor.ExecContainer(ctx, primary, psqlCommand("SHOW archive_mode"))
	if err != nil {
		return fmt.Errorf("show archive_mode: %w", err)
	}
	command, err := executor.ExecContainer(ctx, primary, psqlCommand("SHOW archive_command"))
	if err != nil {
		return fmt.Errorf("show archive_command: %w", err)
	}

	modeChanged := strings.TrimSpace(mode) != "off"
	commandChanged := strings.TrimSpace(command) != ""
	if modeChanged {
		if _, err := executor.ExecContainer(ctx, primary, psqlCommand("ALTER SYSTEM SET archive_mode = 'off'")); err != nil {
			return fmt.Errorf("disable archive_mode: %w", err)
		}
	}
	if commandChanged {
		if _, err := executor.ExecContainer(ctx, primary, psqlCommand("ALTER SYSTEM RESET archive_command")); err != nil {
			return fmt.Errorf("reset archive_command: %w", err)
		}
	}
	if modeChanged {
		if err := executor.RestartContainer(ctx, primary); err != nil {
			return fmt.Errorf("restart primary after disabling archive_mode: %w", err)
		}
	} else if commandChanged {
		if _, err := executor.ExecContainer(ctx, primary, psqlCommand("SELECT pg_reload_conf()")); err != nil {
			return fmt.Errorf("reload archive_command reset: %w", err)
		}
	}
	return nil
}

func psqlCommand(query string) []string {
	return []string{"psql", "-U", postgresUser, "-Atqc", query}
}

func quotePostgresConfig(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
