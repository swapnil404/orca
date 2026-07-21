package pgbouncer

import (
	"context"
	"fmt"
)

// ConsoleExecutor runs the command used to connect to PgBouncer's local admin console.
type ConsoleExecutor interface {
	ExecContainer(ctx context.Context, containerID string, command []string) (string, error)
}

// ReloadConfig asks a running PgBouncer process to reload its configuration.
func ReloadConfig(ctx context.Context, executor ConsoleExecutor, containerID string) error {
	_, err := executor.ExecContainer(ctx, containerID, []string{
		"psql",
		"-v", "ON_ERROR_STOP=1",
		"-h", "/tmp",
		"-p", "6432",
		"-U", "pgbouncer",
		"-d", "pgbouncer",
		"-c", "RELOAD;",
	})
	if err != nil {
		return fmt.Errorf("reload PgBouncer through admin console: %w", err)
	}
	return nil
}
