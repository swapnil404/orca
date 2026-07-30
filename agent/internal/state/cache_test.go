package state

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"
)

func TestFileCachePersistsLatestSnapshotAcrossInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "desired.json")
	cache := NewFileCache(path)
	for _, snapshot := range []*DesiredState{
		{Revision: "first", Clusters: []*ClusterSpec{{Id: "removed", Version: "17"}, {Id: "retained", Version: "17"}}},
		{Revision: "latest", Clusters: []*ClusterSpec{{Id: "retained", Version: "18"}}},
		{Revision: "empty"},
	} {
		if err := cache.Save(context.Background(), snapshot); err != nil {
			t.Fatal(err)
		}
		got, err := NewFileCache(path).Load(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !proto.Equal(snapshot, got) {
			t.Fatalf("loaded %v, want %v", got, snapshot)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("cache permissions = %o, want 600", info.Mode().Perm())
	}
	entries, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".desired-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary files remain: %v", entries)
	}
}

func TestFileCacheMissingAndCorruptFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "desired.json")
	cache := NewFileCache(path)
	got, err := cache.Load(context.Background())
	if err != nil || !proto.Equal(got, &DesiredState{}) {
		t.Fatalf("missing cache = %v, %v", got, err)
	}
	if err := os.WriteFile(path, []byte("{invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Load(context.Background()); err == nil {
		t.Fatal("corrupt cache was silently accepted")
	}
}

func TestFileCacheCancelledSavePreservesPreviousSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "desired.json")
	cache := NewFileCache(path)
	original := &DesiredState{Revision: "previous"}
	if err := cache.Save(context.Background(), original); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := cache.Save(ctx, &DesiredState{Revision: "replacement"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("save error = %v", err)
	}
	if _, err := cache.Load(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("load error = %v", err)
	}
	got, err := cache.Load(context.Background())
	if err != nil || !proto.Equal(got, original) {
		t.Fatalf("previous state changed: %v, %v", got, err)
	}
}

func TestFileCacheReportsFilesystemFailure(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(parent, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	cache := NewFileCache(filepath.Join(parent, "desired.json"))
	if err := cache.Save(context.Background(), &DesiredState{}); err == nil {
		t.Fatal("save accepted invalid parent")
	}
	if _, err := cache.Load(context.Background()); err == nil {
		t.Fatal("load hid filesystem error")
	}
}
