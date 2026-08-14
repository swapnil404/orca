package postgres

import (
	"bufio"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/swapnil404/orca/pkg/postgresconfig"
)

const (
	configApplyTimeout  = 30 * time.Second
	configApplyInterval = 250 * time.Millisecond
	primaryStablePeriod = time.Second
)

// WaitForPrimaryReady waits for the final PostgreSQL postmaster to remain
// available, avoiding the temporary server used by the image entrypoint.
func WaitForPrimaryReady(ctx context.Context, executor ConfigExecutor, containerID string) error {
	if executor == nil {
		return errors.New("config executor is nil")
	}
	command := []string{"psql", "--username", "postgres", "--dbname", "postgres", "--tuples-only", "--no-align", "--command", "SELECT pg_postmaster_start_time();"}
	timer := time.NewTimer(configApplyTimeout)
	defer timer.Stop()
	ticker := time.NewTicker(configApplyInterval)
	defer ticker.Stop()

	var postmaster string
	var stableSince time.Time
	var lastErr error
	for {
		output, err := executor.ExecContainer(ctx, containerID, command)
		output = strings.TrimSpace(output)
		if err == nil && output != "" {
			if output != postmaster {
				postmaster = output
				stableSince = time.Now()
			} else if time.Since(stableSince) >= primaryStablePeriod {
				return nil
			}
			lastErr = nil
		} else {
			postmaster = ""
			stableSince = time.Time{}
			lastErr = err
		}

		select {
		case <-ctx.Done():
			return errors.Join(ctx.Err(), lastErr)
		case <-timer.C:
			return fmt.Errorf("wait for stable PostgreSQL primary: %w", lastErr)
		case <-ticker.C:
		}
	}
}

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

// ParameterMetadata is PostgreSQL's live metadata for one configuration parameter.
type ParameterMetadata struct {
	Name           string
	Context        string
	Setting        string
	Unit           string
	PendingRestart bool
	Applied        bool
	FileSetting    string
}

// RenderConfig renders a deterministic include file that preserves the image's
// generated postgresql.conf and applies Orca-managed overrides afterward.
func RenderConfig(clusterID string, params map[string]string) (string, error) {
	return RenderNodeConfig(clusterID, orcaDataPath(clusterID)+"/primary", params)
}

// RenderNodeConfig renders an Orca-managed configuration file for one PostgreSQL data directory.
func RenderNodeConfig(clusterID, dataPath string, params map[string]string) (string, error) {
	if clusterID == "" || strings.ContainsAny(clusterID, `/\\`) {
		return "", fmt.Errorf("cluster ID must be a path segment")
	}
	if dataPath == "" || !strings.HasPrefix(dataPath, orcaDataPath(clusterID)+"/") {
		return "", fmt.Errorf("PostgreSQL data path must belong to cluster %q", clusterID)
	}

	values, err := postgresconfig.ValidateParameters(params)
	if err != nil {
		return "", err
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var config strings.Builder
	fmt.Fprintf(&config, "include = '%s/postgresql.conf'\n", dataPath)
	for _, key := range keys {
		fmt.Fprintf(&config, "%s = '%s'\n", key, strings.ReplaceAll(values[key], "'", "''"))
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
		if !found || !postgresconfig.ValidParameterName(key) || len(value) < 2 || value[0] != '\'' || value[len(value)-1] != '\'' {
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
	return postgresconfig.ChangedParameters(desired, applied)
}

// ClassifyConfigUpdate determines whether changed parameters can be reloaded
// or require a PostgreSQL process restart using pg_settings metadata.
func ClassifyConfigUpdate(changed []string, metadata map[string]ParameterMetadata) (ConfigUpdateMethod, error) {
	method := ConfigUpdateReload
	for _, parameter := range changed {
		name := postgresconfig.NormalizeParameterName(parameter)
		setting, exists := metadata[name]
		if !exists {
			return "", fmt.Errorf("unknown PostgreSQL parameter %q", name)
		}
		if setting.Context == "internal" {
			return "", fmt.Errorf("PostgreSQL parameter %q is internal and cannot be configured", name)
		}
		if setting.Context == "postmaster" {
			method = ConfigUpdateRestart
		}
	}
	return method, nil
}

// InspectParameters reads version-specific parameter metadata from pg_settings.
func InspectParameters(ctx context.Context, executor ConfigExecutor, containerID string, names []string) (map[string]ParameterMetadata, error) {
	if executor == nil {
		return nil, fmt.Errorf("config executor is nil")
	}
	requested := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = postgresconfig.NormalizeParameterName(name)
		if !postgresconfig.ValidParameterName(name) {
			return nil, fmt.Errorf("invalid PostgreSQL parameter name %q", name)
		}
		requested[name] = struct{}{}
	}
	ordered := make([]string, 0, len(requested))
	for name := range requested {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	literals := make([]string, len(ordered))
	for index, name := range ordered {
		literals[index] = "'" + name + "'"
	}
	requestedCondition := "FALSE"
	if len(literals) > 0 {
		requestedCondition = "name IN (" + strings.Join(literals, ",") + ")"
	}
	query := fmt.Sprintf("COPY (SELECT name, context, setting, COALESCE(unit, ''), pending_restart, EXISTS (SELECT 1 FROM pg_file_settings f WHERE f.sourcefile = '/etc/orca/postgresql.conf' AND f.name = pg_settings.name AND f.applied AND f.error IS NULL), COALESCE((SELECT f.setting FROM pg_file_settings f WHERE f.sourcefile = '/etc/orca/postgresql.conf' AND f.name = pg_settings.name ORDER BY f.seqno DESC LIMIT 1), '') FROM pg_settings WHERE %s OR name IN (SELECT f.name FROM pg_file_settings f WHERE f.sourcefile = '/etc/orca/postgresql.conf') ORDER BY name) TO STDOUT WITH (FORMAT CSV)", requestedCondition)
	output, err := executor.ExecContainer(ctx, containerID, []string{"psql", "--username", "postgres", "--dbname", "postgres", "--command", query})
	if err != nil {
		return nil, fmt.Errorf("query pg_settings: %w", err)
	}
	metadata, err := parseParameterMetadata(output)
	if err != nil {
		return nil, err
	}
	for _, name := range ordered {
		if _, exists := metadata[name]; !exists {
			return metadata, fmt.Errorf("unknown PostgreSQL parameter %q", name)
		}
	}
	return metadata, nil
}

func parseParameterMetadata(output string) (map[string]ParameterMetadata, error) {
	metadata := make(map[string]ParameterMetadata)
	reader := csv.NewReader(strings.NewReader(output))
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse pg_settings output: %w", err)
		}
		if len(record) != 7 {
			return nil, fmt.Errorf("parse pg_settings output: expected 7 fields, got %d", len(record))
		}
		metadata[record[0]] = ParameterMetadata{
			Name: record[0], Context: record[1], Setting: record[2], Unit: record[3], PendingRestart: record[4] == "t", Applied: record[5] == "t", FileSetting: record[6],
		}
	}
	return metadata, nil
}

// ValidateParameterValues asks the target PostgreSQL binary to parse the complete candidate without starting another server.
func ValidateParameterValues(ctx context.Context, executor ConfigExecutor, containerID, dataPath string, params map[string]string) error {
	params, err := postgresconfig.ValidateParameters(params)
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(params))
	for name := range params {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return nil
	}
	command := []string{"gosu", "postgres", "postgres", "-D", dataPath, "-C", keys[0]}
	for _, name := range keys {
		command = append(command, "-c", name+"="+params[name])
	}
	if _, err := executor.ExecContainer(ctx, containerID, command); err != nil {
		return fmt.Errorf("invalid PostgreSQL parameter values: %w", err)
	}
	return nil
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
