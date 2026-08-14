package extensions

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/swapnil404/orca/agent/internal/postgres"
)

const installedExtensionsQuery = "SELECT extname || E'\\t' || extversion FROM pg_extension WHERE extname IN ('vector', 'powa', 'timescaledb', 'pg_partman', 'postgis') ORDER BY extname;"

const sharedPreloadLibrariesQuery = "SHOW shared_preload_libraries;"

// PrimaryExecutor runs PostgreSQL commands and restarts a cluster primary.
type PrimaryExecutor interface {
	ExecContainer(ctx context.Context, containerID string, command []string) (string, error)
	RestartContainer(ctx context.Context, containerID string) error
}

// Result reports the outcome of one extension action.
type Result struct {
	Action Action
	Err    error
}

// InstalledDetails queries managed extension names and their observed versions.
func InstalledDetails(ctx context.Context, executor PrimaryExecutor, containerID string) (map[string]string, error) {
	if executor == nil {
		return nil, fmt.Errorf("extension executor is nil")
	}
	output, err := executor.ExecContainer(ctx, containerID, psqlCommand(installedExtensionsQuery))
	if err != nil {
		return nil, fmt.Errorf("query installed extensions: %w", err)
	}
	return parseInstalledDetails(output)
}

// Apply executes extension actions, batching preload changes into one restart.
func Apply(ctx context.Context, executor PrimaryExecutor, containerID string, desired []string, actions []Action) []Result {
	errorsByAction := make(map[Action]error, len(actions))
	restartActions := make([]Action, 0)
	for _, action := range actions {
		if action.Method == UpdateMethodRestart {
			restartActions = append(restartActions, action)
			continue
		}
		errorsByAction[action] = executeAction(ctx, executor, containerID, action)
	}

	if len(restartActions) == 0 {
		return resultsFor(actions, errorsByAction)
	}

	preservePreloads := make([]string, 0)
	for _, action := range restartActions {
		if action.Type != ActionDrop {
			continue
		}
		errorsByAction[action] = executeAction(ctx, executor, containerID, action)
		if errorsByAction[action] != nil {
			preservePreloads = append(preservePreloads, action.Extension)
		}
	}

	batchErr := applyPreloadAndRestart(ctx, executor, containerID, desired, preservePreloads)
	for _, action := range restartActions {
		if batchErr != nil {
			errorsByAction[action] = errors.Join(errorsByAction[action], batchErr)
			continue
		}
		if action.Type == ActionCreate {
			errorsByAction[action] = executeAction(ctx, executor, containerID, action)
		}
	}

	return resultsFor(actions, errorsByAction)
}

func resultsFor(actions []Action, errorsByAction map[Action]error) []Result {
	results := make([]Result, 0, len(actions))
	for _, action := range actions {
		results = append(results, Result{Action: action, Err: errorsByAction[action]})
	}
	return results
}

func applyPreloadAndRestart(ctx context.Context, executor PrimaryExecutor, containerID string, desired, preserve []string) error {
	output, err := executor.ExecContainer(ctx, containerID, psqlCommand(sharedPreloadLibrariesQuery))
	if err != nil {
		return fmt.Errorf("query shared_preload_libraries: %w", err)
	}
	current := preloadLibrarySet(output)
	target := make(map[string]struct{}, len(current))
	for library := range current {
		target[library] = struct{}{}
	}
	delete(target, sqlNames["powa"])
	delete(target, sqlNames["timescaledb"])
	for _, extension := range desired {
		if restartRequiredExtensions[extension] {
			target[sqlNames[extension]] = struct{}{}
		}
	}
	for _, extension := range preserve {
		target[sqlNames[extension]] = struct{}{}
	}

	libraries := make([]string, 0, len(target))
	for library := range target {
		libraries = append(libraries, library)
	}
	sort.Strings(libraries)
	query := "ALTER SYSTEM RESET shared_preload_libraries;"
	if len(libraries) > 0 {
		query = "ALTER SYSTEM SET shared_preload_libraries = " + quoteConfig(strings.Join(libraries, ",")) + ";"
	}
	if _, err := executor.ExecContainer(ctx, containerID, psqlCommand(query)); err != nil {
		return fmt.Errorf("configure shared_preload_libraries: %w", err)
	}
	if err := executor.RestartContainer(ctx, containerID); err != nil {
		return fmt.Errorf("restart primary for extension changes: %w", err)
	}
	if err := postgres.WaitForPrimaryReady(ctx, executor, containerID); err != nil {
		return fmt.Errorf("wait for primary after extension restart: %w", err)
	}
	return nil
}

func executeAction(ctx context.Context, executor PrimaryExecutor, containerID string, action Action) error {
	_, err := executor.ExecContainer(ctx, containerID, psqlCommand(statement(action)))
	if err != nil {
		return fmt.Errorf("%s extension %q: %w", action.Type, action.Extension, err)
	}
	return nil
}

func preloadLibrarySet(output string) map[string]struct{} {
	libraries := make(map[string]struct{})
	for _, library := range strings.Split(output, ",") {
		library = strings.TrimSpace(library)
		if library != "" {
			libraries[library] = struct{}{}
		}
	}
	return libraries
}

func parseInstalledDetails(output string) (map[string]string, error) {
	if strings.TrimSpace(output) == "" {
		return map[string]string{}, nil
	}

	reverseNames := make(map[string]string, len(sqlNames))
	for extension, sqlName := range sqlNames {
		reverseNames[sqlName] = extension
	}
	lines := strings.Split(output, "\n")
	installed := make(map[string]string, len(lines))
	for _, line := range lines {
		parts := strings.SplitN(strings.TrimSpace(line), "\t", 2)
		sqlName := parts[0]
		extension, supported := reverseNames[sqlName]
		if !supported {
			return nil, fmt.Errorf("query returned unsupported extension %q", sqlName)
		}
		version := ""
		if len(parts) == 2 {
			version = strings.TrimSpace(parts[1])
		}
		installed[extension] = version
	}
	return installed, nil
}

func statement(action Action) string {
	sqlName := sqlNames[action.Extension]
	if action.Type == ActionCreate {
		return "CREATE EXTENSION IF NOT EXISTS " + sqlName + ";"
	}
	return "DROP EXTENSION IF EXISTS " + sqlName + ";"
}

func quoteConfig(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
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
