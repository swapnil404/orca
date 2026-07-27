package pgbouncer

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/swapnil404/orca/pkg/types"
)

// ConsoleExecutor runs the command used to connect to PgBouncer's local admin console.
type ConsoleExecutor interface {
	ExecContainer(ctx context.Context, containerID string, command []string) (string, error)
}

// ReloadConfig asks a running PgBouncer process to reload its configuration.
func ReloadConfig(ctx context.Context, executor ConsoleExecutor, containerID string) error {
	_, err := executor.ExecContainer(ctx, containerID, consoleCommand("RELOAD;"))
	if err != nil {
		return fmt.Errorf("reload PgBouncer through admin console: %w", err)
	}
	return nil
}

// PopulateStatus records admin-console reachability and live client capacity.
func PopulateStatus(ctx context.Context, executor ConsoleExecutor, actual *types.ActualState) {
	if executor == nil || actual == nil {
		return
	}
	for _, cluster := range actual.Clusters {
		pool := cluster.GetPgBouncer()
		if pool == nil || pool.ContainerId == "" || pool.Status != "running" {
			continue
		}
		reachable := false
		output, err := executor.ExecContainer(ctx, pool.ContainerId, consoleCommand("SHOW POOLS;"))
		if err == nil {
			if active, parseErr := activeConnections(output); parseErr == nil {
				reachable = true
				pool.ActiveClientConnections = &active
			}
		}
		pool.AdminConsoleReachable = &reachable
		if !reachable {
			continue
		}
		output, err = executor.ExecContainer(ctx, pool.ContainerId, consoleCommand("SHOW CONFIG;"))
		if err == nil {
			if maximum, parseErr := maxConnections(output); parseErr == nil {
				pool.MaxClientConnections = &maximum
			}
		}
	}
}

func consoleCommand(query string) []string {
	return []string{
		"psql",
		"-v", "ON_ERROR_STOP=1",
		"-At", "-F", "|",
		"-h", "/tmp",
		"-p", "6432",
		"-U", "pgbouncer",
		"-d", "pgbouncer",
		"-c", query,
	}
}

func activeConnections(output string) (uint32, error) {
	var total uint64
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, "|")
		if len(fields) < 10 {
			return 0, fmt.Errorf("parse SHOW POOLS row: got %d fields", len(fields))
		}
		active, err := strconv.ParseUint(strings.TrimSpace(fields[2]), 10, 32)
		if err != nil {
			return 0, fmt.Errorf("parse SHOW POOLS active clients: %w", err)
		}
		total += active
		if total > uint64(^uint32(0)) {
			return 0, fmt.Errorf("parse SHOW POOLS active clients: overflow")
		}
	}
	return uint32(total), nil
}

func maxConnections(output string) (uint32, error) {
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		fields := strings.Split(line, "|")
		if len(fields) < 2 || strings.TrimSpace(fields[0]) != "max_client_conn" {
			continue
		}
		value, err := strconv.ParseUint(strings.TrimSpace(fields[1]), 10, 32)
		if err != nil || value == 0 {
			return 0, fmt.Errorf("parse SHOW CONFIG max_client_conn")
		}
		return uint32(value), nil
	}
	return 0, fmt.Errorf("parse SHOW CONFIG: max_client_conn not found")
}
