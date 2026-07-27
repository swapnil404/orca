package postgres

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	configApplyTimeout  = 30 * time.Second
	configApplyInterval = 250 * time.Millisecond
)

// ConfigExecutor runs commands in a PostgreSQL container.
type ConfigExecutor interface {
	ExecContainer(context.Context, string, []string) (string, error)
}

// ConfigUpdateMethod identifies how changed PostgreSQL parameters are applied.
type ConfigUpdateMethod string

const (
	// ConfigUpdateReload applies parameters with pg_reload_conf.
	ConfigUpdateReload ConfigUpdateMethod = "reload"
	// ConfigUpdateRestart applies parameters by restarting PostgreSQL.
	ConfigUpdateRestart ConfigUpdateMethod = "restart"
)

var parameterNamePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]*$`)

// PostgreSQL documents these parameters as start-time settings. Keep this
// explicit so adding support for a new parameter has a reviewable lifecycle.
var restartRequiredParameters = map[string]struct{}{
	"allow_alter_system": {}, "archive_mode": {}, "autovacuum_max_workers": {},
	"autovacuum_freeze_max_age": {}, "autovacuum_multixact_freeze_max_age": {},
	"autovacuum_worker_slots": {}, "bonjour": {}, "bonjour_name": {},
	"cluster_name": {}, "config_file": {}, "data_directory": {}, "data_sync_retry": {},
	"debug_io_direct": {}, "dynamic_shared_memory_type": {}, "event_source": {},
	"external_pid_file": {}, "hba_file": {}, "hot_standby": {}, "huge_pages": {},
	"huge_page_size": {}, "ident_file": {}, "ignore_invalid_pages": {},
	"jit_provider": {}, "listen_addresses": {}, "logging_collector": {}, "max_connections": {},
	"max_files_per_process": {}, "max_locks_per_transaction": {},
	"max_logical_replication_workers": {}, "max_pred_locks_per_transaction": {},
	"max_prepared_transactions": {}, "max_replication_slots": {},
	"max_wal_senders": {}, "max_worker_processes": {}, "min_dynamic_shared_memory": {},
	"old_snapshot_threshold": {}, "port": {}, "recovery_target": {},
	"recovery_target_action": {}, "recovery_target_inclusive": {},
	"recovery_target_lsn": {}, "recovery_target_name": {}, "recovery_target_time": {},
	"recovery_target_timeline": {}, "recovery_target_xid": {}, "reserved_connections": {},
	"shared_buffers": {}, "shared_memory_type": {}, "shared_preload_libraries": {},
	"superuser_reserved_connections": {}, "track_activity_query_size": {},
	"track_commit_timestamp": {}, "unix_socket_directories": {},
	"unix_socket_group": {}, "unix_socket_permissions": {}, "wal_buffers": {},
	"wal_decode_buffer_size": {}, "wal_level": {}, "wal_log_hints": {},
}

// RenderConfig renders a deterministic include file that preserves the image's
// generated postgresql.conf and applies Orca-managed overrides afterward.
func RenderConfig(clusterID string, params map[string]string) (string, error) {
	if clusterID == "" || strings.ContainsAny(clusterID, `/\\`) {
		return "", fmt.Errorf("cluster ID must be a path segment")
	}

	keys := make([]string, 0, len(params))
	for key := range params {
		if !parameterNamePattern.MatchString(key) {
			return "", fmt.Errorf("invalid PostgreSQL parameter name %q", key)
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var config strings.Builder
	fmt.Fprintf(&config, "include = '%s/primary/postgresql.conf'\n", orcaDataPath(clusterID))
	for _, key := range keys {
		if strings.ContainsAny(params[key], "\r\n") {
			return "", fmt.Errorf("PostgreSQL parameter %q contains a newline", key)
		}
		fmt.Fprintf(&config, "%s = '%s'\n", key, strings.ReplaceAll(params[key], "'", "''"))
	}
	return config.String(), nil
}

// ParseConfig reads parameters from an Orca-generated PostgreSQL include file.
func ParseConfig(config string) (map[string]string, error) {
	if config == "" {
		return nil, nil
	}
	params := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(config))
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "include = ") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if !found || !parameterNamePattern.MatchString(key) || len(value) < 2 || value[0] != '\'' || value[len(value)-1] != '\'' {
			return nil, fmt.Errorf("invalid generated PostgreSQL config line %d", lineNumber)
		}
		params[key] = strings.ReplaceAll(value[1:len(value)-1], "''", "'")
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read generated PostgreSQL config: %w", err)
	}
	return params, nil
}

// ChangedParameters returns parameter names whose desired and applied values differ.
func ChangedParameters(desired, applied map[string]string) []string {
	changed := make(map[string]struct{})
	for key, value := range desired {
		if appliedValue, exists := applied[key]; !exists || appliedValue != value {
			changed[strings.ToLower(strings.TrimSpace(key))] = struct{}{}
		}
	}
	for key := range applied {
		if _, exists := desired[key]; !exists {
			changed[strings.ToLower(strings.TrimSpace(key))] = struct{}{}
		}
	}

	result := make([]string, 0, len(changed))
	for key := range changed {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

// ClassifyConfigUpdate determines whether changed parameters can be reloaded
// or require a PostgreSQL process restart.
func ClassifyConfigUpdate(changed []string) ConfigUpdateMethod {
	for _, parameter := range changed {
		if _, restartRequired := restartRequiredParameters[strings.ToLower(strings.TrimSpace(parameter))]; restartRequired {
			return ConfigUpdateRestart
		}
	}
	return ConfigUpdateReload
}

// WaitForConfigApplied waits for PostgreSQL to accept every setting from the
// Orca-managed configuration file without a later file overriding it.
func WaitForConfigApplied(ctx context.Context, executor ConfigExecutor, containerID string, expected int) error {
	if executor == nil {
		return fmt.Errorf("config executor is nil")
	}
	query := fmt.Sprintf(
		"SELECT count(*) = %d AND COALESCE(bool_and(applied AND error IS NULL), true) FROM pg_file_settings WHERE sourcefile = '%s'",
		expected, "/etc/orca/postgresql.conf",
	)
	command := []string{"psql", "--username", "postgres", "--dbname", "postgres", "--tuples-only", "--no-align", "--command", query}
	timer := time.NewTimer(configApplyTimeout)
	defer timer.Stop()
	ticker := time.NewTicker(configApplyInterval)
	defer ticker.Stop()

	var lastErr error
	for {
		output, err := executor.ExecContainer(ctx, containerID, command)
		if err == nil && strings.TrimSpace(output) == "t" {
			return nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("PostgreSQL did not apply every managed parameter")
		}

		select {
		case <-ctx.Done():
			return errors.Join(ctx.Err(), lastErr)
		case <-timer.C:
			return fmt.Errorf("wait for PostgreSQL config: %w", lastErr)
		case <-ticker.C:
		}
	}
}

func orcaDataPath(clusterID string) string {
	return "/var/orca/data/" + clusterID
}
