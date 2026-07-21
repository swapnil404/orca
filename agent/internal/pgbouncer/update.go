package pgbouncer

import (
	"bufio"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// UpdateMethod identifies how a PgBouncer configuration change is applied.
type UpdateMethod string

const (
	// UpdateMethodReload applies configuration through the admin console.
	UpdateMethodReload UpdateMethod = "reload"
	// UpdateMethodRestart applies configuration by restarting PgBouncer.
	UpdateMethodRestart UpdateMethod = "restart"
)

// ConfigKey identifies one setting within a PgBouncer INI section.
type ConfigKey struct {
	Section string
	Name    string
}

var restartRequiredConfigKeys = map[ConfigKey]struct{}{
	{Section: "pgbouncer", Name: "auth_type"}:         {},
	{Section: "pgbouncer", Name: "job_name"}:          {},
	{Section: "pgbouncer", Name: "listen_addr"}:       {},
	{Section: "pgbouncer", Name: "listen_port"}:       {},
	{Section: "pgbouncer", Name: "pidfile"}:           {},
	{Section: "pgbouncer", Name: "service_name"}:      {},
	{Section: "pgbouncer", Name: "so_reuseport"}:      {},
	{Section: "pgbouncer", Name: "unix_socket_dir"}:   {},
	{Section: "pgbouncer", Name: "unix_socket_group"}: {},
	{Section: "pgbouncer", Name: "unix_socket_mode"}:  {},
	{Section: "pgbouncer", Name: "user"}:              {},
}

// ClassifyConfigUpdate determines whether changed configuration keys can be
// reloaded or require a process restart.
func ClassifyConfigUpdate(changed []ConfigKey) UpdateMethod {
	for _, key := range changed {
		key.Section = strings.ToLower(strings.TrimSpace(key.Section))
		key.Name = strings.ToLower(strings.TrimSpace(key.Name))
		if _, restartRequired := restartRequiredConfigKeys[key]; restartRequired {
			return UpdateMethodRestart
		}
	}
	return UpdateMethodReload
}

// ChangedConfigKeys returns the sorted keys whose values differ between two
// complete PgBouncer INI files.
func ChangedConfigKeys(oldConfig, newConfig string) ([]ConfigKey, error) {
	oldValues, err := parseConfigValues(oldConfig)
	if err != nil {
		return nil, fmt.Errorf("parse old PgBouncer config: %w", err)
	}
	newValues, err := parseConfigValues(newConfig)
	if err != nil {
		return nil, fmt.Errorf("parse new PgBouncer config: %w", err)
	}

	changed := make(map[ConfigKey]struct{})
	for key, oldValue := range oldValues {
		if newValue, exists := newValues[key]; !exists || newValue != oldValue {
			changed[key] = struct{}{}
		}
	}
	for key, newValue := range newValues {
		if oldValue, exists := oldValues[key]; !exists || oldValue != newValue {
			changed[key] = struct{}{}
		}
	}

	keys := make([]ConfigKey, 0, len(changed))
	for key := range changed {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Section != keys[j].Section {
			return keys[i].Section < keys[j].Section
		}
		return keys[i].Name < keys[j].Name
	})
	return keys, nil
}

func parseConfigValues(config string) (map[ConfigKey]string, error) {
	values := make(map[ConfigKey]string)
	section := ""
	scanner := bufio.NewScanner(strings.NewReader(config))
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			if section == "" {
				return nil, fmt.Errorf("line %d has an empty section", lineNumber)
			}
			continue
		}
		if section == "" {
			return nil, fmt.Errorf("line %d is outside a section", lineNumber)
		}
		name, value, found := strings.Cut(line, "=")
		name = strings.ToLower(strings.TrimSpace(name))
		if !found || name == "" {
			return nil, fmt.Errorf("line %d is not a key-value setting", lineNumber)
		}
		key := ConfigKey{Section: section, Name: name}
		if _, duplicate := values[key]; duplicate {
			return nil, fmt.Errorf("line %d duplicates %s.%s", lineNumber, section, name)
		}
		values[key] = strings.TrimSpace(value)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, errors.New("config contains no settings")
	}
	return values, nil
}
