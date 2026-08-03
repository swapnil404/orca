package pgbackrest

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/swapnil404/orca/pkg/types"
	"google.golang.org/protobuf/proto"
)

func TestRestoreJournalSurvivesManagerRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "desired.json")
	original := NewRestoreManager(path, nil)
	report := &types.RestoreOperationReport{OperationId: "operation", Sequence: 7, Status: "running", Phase: "restore", DestructiveStarted: true, RollbackAvailable: true}
	original.journal.Operations["operation"] = &restoreRecord{Fingerprint: "fingerprint", Mode: "in_place", Source: "alpha", Report: report}
	if err := original.save(context.Background()); err != nil {
		t.Fatal(err)
	}
	reloaded := NewRestoreManager(path, nil)
	if err := reloaded.load(); err != nil {
		t.Fatal(err)
	}
	got := reloaded.journal.Operations["operation"]
	if got == nil || got.Fingerprint != "fingerprint" || !proto.Equal(got.Report, report) {
		t.Fatalf("recovery checkpoint lost: %+v", got)
	}
	info, err := os.Stat(original.journalPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("journal permissions = %o", info.Mode().Perm())
	}
}

func TestRestoreJournalRejectsCorruptionAndUnsupportedVersion(t *testing.T) {
	for _, data := range []string{"{invalid", `{"version":2,"operations":{}}`, `{"version":1,"operations":{"bad":null}}`, `{"version":1,"operations":{"bad":{}}}`} {
		t.Run(data, func(t *testing.T) {
			manager := NewRestoreManager(filepath.Join(t.TempDir(), "desired.json"), nil)
			if err := os.WriteFile(manager.journalPath, []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := manager.load(); err == nil {
				t.Fatal("unsafe journal accepted")
			}
		})
	}
}

func TestCancelledRestoreJournalSavePreservesCheckpoint(t *testing.T) {
	manager := NewRestoreManager(filepath.Join(t.TempDir(), "desired.json"), nil)
	if err := manager.save(context.Background()); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(manager.journalPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := manager.save(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error = %v", err)
	}
	after, err := os.ReadFile(manager.journalPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("cancelled write changed checkpoint")
	}
}
