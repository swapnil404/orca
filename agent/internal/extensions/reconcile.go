package extensions

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const installedExtensionsQuery = "SELECT extname FROM pg_extension WHERE extname IN ('vector', 'powa', 'timescaledb', 'pg_partman', 'postgis') ORDER BY extname;"

const (
	sharedPreloadLibrariesQuery = "SHOW shared_preload_libraries;"
	readinessQuery              = "SELECT 1;"
	readinessPollInterval       = 250 * time.Millisecond
	readinessTimeout            = 30 * time.Second
)

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

// Reconcile queries installed extensions and applies the desired changes.
func Reconcile(ctx context.Context, executor PrimaryExecutor, containerID string, desired []string) ([]Result, error) {
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

	errorsByAction := make(map[Action]error, len(actions))
	restartActions := make([]Action, 0)
	for _, action := range actions {
		if action.Method == UpdateMethodRestart {
			restartActions = append(restartActions, action)
			continue
		}
		errorsByAction[action] = executeAction(ctx, executor, containerID, action)
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
	if len(restartActions) > 0 {
		for _, action := range restartActions {
			if batchErr != nil {
				errorsByAction[action] = errors.Join(errorsByAction[action], batchErr)
				continue
			}
			if action.Type == ActionCreate {
				errorsByAction[action] = executeAction(ctx, executor, containerID, action)
			}
		}
	}

	results := make([]Result, 0, len(actions))
	for _, action := range actions {
		results = append(results, Result{Action: action, Err: errorsByAction[action]})
	}
	if len(restartActions) == 0 && batchErr != nil {
		return results, batchErr
	}
	return results, nil
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
	delete(target, definitions["powa"].SQLName)
	delete(target, definitions["timescaledb"].SQLName)
	for _, extension := range desired {
		definition, supported := definitions[extension]
		if supported && definition.RequiresRestart {
			target[definition.SQLName] = struct{}{}
		}
	}
	for _, extension := range preserve {
		target[definitions[extension].SQLName] = struct{}{}
	}
	if equalSets(current, target) {
		return nil
	}

	libraries := make([]string, 0, len(target))
	for library := range target {
		libraries = append(libraries, library)
	}
	sort.Strings(libraries)
	query := "ALTER SYSTEM SET shared_preload_libraries = " + quoteConfig(strings.Join(libraries, ",")) + ";"
	if _, err := executor.ExecContainer(ctx, containerID, psqlCommand(query)); err != nil {
		return fmt.Errorf("configure shared_preload_libraries: %w", err)
	}
	if err := executor.RestartContainer(ctx, containerID); err != nil {
		return fmt.Errorf("restart primary for extension changes: %w", err)
	}
	if err := waitUntilReady(ctx, executor, containerID); err != nil {
		return fmt.Errorf("wait for primary after extension restart: %w", err)
	}
	return nil
}

func equalSets(left, right map[string]struct{}) bool {
	if len(left) != len(right) {
		return false
	}
	for value := range left {
		if _, exists := right[value]; !exists {
			return false
		}
	}
	return true
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

func waitUntilReady(ctx context.Context, executor PrimaryExecutor, containerID string) error {
	readyCtx, cancel := context.WithTimeout(ctx, readinessTimeout)
	defer cancel()
	ticker := time.NewTicker(readinessPollInterval)
	defer ticker.Stop()

	for {
		output, err := executor.ExecContainer(readyCtx, containerID, psqlCommand(readinessQuery))
		if err == nil && strings.TrimSpace(output) == "1" {
			return nil
		}
		select {
		case <-readyCtx.Done():
			if errors.Is(readyCtx.Err(), context.DeadlineExceeded) {
				return errors.New("timed out waiting for PostgreSQL readiness")
			}
			return readyCtx.Err()
		case <-ticker.C:
		}
	}
}

func parseInstalled(output string) ([]string, error) {
	if strings.TrimSpace(output) == "" {
		return nil, nil
	}

	reverseNames := make(map[string]string, len(definitions))
	for extension, definition := range definitions {
		reverseNames[definition.SQLName] = extension
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
	sqlName := definitions[action.Extension].SQLName
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
