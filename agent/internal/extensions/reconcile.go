package extensions

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

const installedExtensionsQuery = "SELECT extname FROM pg_extension WHERE extname IN ('vector', 'powa', 'timescaledb', 'pg_partman', 'postgis') ORDER BY extname;"

// ContainerExecutor runs PostgreSQL commands in a cluster primary.
type ContainerExecutor interface {
	ExecContainer(ctx context.Context, containerID string, command []string) (string, error)
}

// Result reports the outcome of one extension action.
type Result struct {
	Action Action
	Err    error
}

// Reconcile queries installed extensions and applies the desired changes.
func Reconcile(ctx context.Context, executor ContainerExecutor, containerID string, desired []string) ([]Result, error) {
	if executor == nil {
		return nil, fmt.Errorf("extension executor is nil")
	}
	output, err := executor.ExecContainer(ctx, containerID, psqlCommand(installedExtensionsQuery))
	if err != nil {
		return nil, fmt.Errorf("query installed extensions: %w", err)
	}
	actual, err := parseInstalled(output)
	if err != nil {
		return nil, err
	}
	actions, err := Diff(desired, actual)
	if err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(actions))
	for _, action := range actions {
		_, actionErr := executor.ExecContainer(ctx, containerID, psqlCommand(statement(action)))
		if actionErr != nil {
			actionErr = fmt.Errorf("%s extension %q: %w", action.Type, action.Extension, actionErr)
		}
		results = append(results, Result{Action: action, Err: actionErr})
	}
	return results, nil
}

func parseInstalled(output string) ([]string, error) {
	if strings.TrimSpace(output) == "" {
		return nil, nil
	}

	reverseNames := make(map[string]string, len(sqlNames))
	for extension, sqlName := range sqlNames {
		reverseNames[sqlName] = extension
	}
	lines := strings.Split(output, "\n")
	installed := make([]string, 0, len(lines))
	for _, line := range lines {
		sqlName := strings.TrimSpace(line)
		extension, supported := reverseNames[sqlName]
		if !supported {
			return nil, fmt.Errorf("query returned unsupported extension %q", sqlName)
		}
		installed = append(installed, extension)
	}
	sort.Strings(installed)
	return installed, nil
}

func statement(action Action) string {
	sqlName := sqlNames[action.Extension]
	if action.Type == ActionCreate {
		return "CREATE EXTENSION IF NOT EXISTS " + sqlName + ";"
	}
	return "DROP EXTENSION IF EXISTS " + sqlName + ";"
}

func psqlCommand(statement string) []string {
	return []string{
		"psql",
		"-v", "ON_ERROR_STOP=1",
		"-U", "postgres",
		"-d", "postgres",
		"-Atqc", statement,
	}
}
