package state

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultPath is the default location of the desired-state cache.
const DefaultPath = "/var/orca/state/desired.json"

// StateCache persists the desired state used by reconciliation.
type StateCache interface {
	Save(ctx context.Context, state DesiredState) error
	Load(ctx context.Context) (DesiredState, error)
}

// FileCache stores desired state as a JSON file.
type FileCache struct {
	path string
}

var _ StateCache = (*FileCache)(nil)

// NewFileCache creates a file-backed cache. An empty path uses DefaultPath.
func NewFileCache(path string) *FileCache {
	if path == "" {
		path = DefaultPath
	}

	return &FileCache{path: path}
}

// Save atomically writes the desired state to disk.
func (c *FileCache) Save(ctx context.Context, state DesiredState) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal desired state: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}

	temporary, err := os.CreateTemp(dir, ".desired-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary state file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("set temporary state file permissions: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("write desired state: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync desired state: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close desired state: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, c.path); err != nil {
		return fmt.Errorf("replace desired state: %w", err)
	}

	return nil
}

// Load reads the desired state from disk. A missing file is an empty state.
func (c *FileCache) Load(ctx context.Context) (DesiredState, error) {
	if err := ctx.Err(); err != nil {
		return DesiredState{}, err
	}

	data, err := os.ReadFile(c.path)
	if errors.Is(err, os.ErrNotExist) {
		return DesiredState{}, nil
	}
	if err != nil {
		return DesiredState{}, fmt.Errorf("read desired state: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return DesiredState{}, err
	}

	var state DesiredState
	if err := json.Unmarshal(data, &state); err != nil {
		return DesiredState{}, fmt.Errorf("decode desired state: %w", err)
	}

	return state, nil
}
