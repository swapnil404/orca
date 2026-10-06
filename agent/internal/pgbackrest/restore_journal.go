package pgbackrest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	orcatypes "github.com/swapnil404/orca/pkg/types"
	"os"
	"path/filepath"
)

type restoreJournal struct {
	Version    int                       `json:"version"`
	Operations map[string]*restoreRecord `json:"operations"`
}

type restoreRecord struct {
	Fingerprint string                            `json:"fingerprint"`
	Mode        string                            `json:"mode"`
	Source      string                            `json:"source"`
	Target      string                            `json:"target,omitempty"`
	TargetTime  string                            `json:"target_time,omitempty"`
	BackupLabel string                            `json:"backup_label,omitempty"`
	Step        int                               `json:"step"`
	SourceSpec  *orcatypes.ClusterSpec            `json:"source_spec,omitempty"`
	TargetSpec  *orcatypes.ClusterSpec            `json:"target_spec,omitempty"`
	Report      *orcatypes.RestoreOperationReport `json:"report"`
}

func (m *RestoreManager) load() error {
	data, err := os.ReadFile(m.journalPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read restore journal: %w", err)
	}
	var journal restoreJournal
	if err := json.Unmarshal(data, &journal); err != nil {
		return fmt.Errorf("decode restore journal: %w", err)
	}
	if journal.Version != 1 {
		return fmt.Errorf("unsupported restore journal version %d", journal.Version)
	}
	if journal.Operations == nil {
		journal.Operations = make(map[string]*restoreRecord)
	}
	for id, record := range journal.Operations {
		if record == nil || record.Report == nil {
			return fmt.Errorf("restore journal operation %q has no report", id)
		}
	}
	m.journal = journal
	return nil
}

func (m *RestoreManager) save(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m.journal, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal restore journal: %w", err)
	}
	data = append(data, '\n')
	dir := filepath.Dir(m.journalPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create restore journal directory: %w", err)
	}
	temporary, err := os.CreateTemp(dir, ".restore-operations-*.tmp")
	if err != nil {
		return fmt.Errorf("create restore journal temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, m.journalPath); err != nil {
		return fmt.Errorf("replace restore journal: %w", err)
	}
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	return errors.Join(syncErr, closeErr)
}
