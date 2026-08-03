package pgbackrest

import (
	"math"
	"testing"
	"time"
)

func TestSelectBackupIgnoresFutureFailedAndIncompleteBackups(t *testing.T) {
	stanza := &restoreInfoStanza{}
	for _, tt := range []struct {
		label  string
		stop   int64
		failed bool
	}{{"older", 50, false}, {"future", 101, false}, {"failed", 99, true}, {"incomplete", 0, false}, {"newest", 90, false}, {"", 100, false}} {
		backup := restoreInfoBackup{Label: tt.label, Error: tt.failed}
		backup.Timestamp.Stop = tt.stop
		stanza.Backup = append(stanza.Backup, backup)
	}
	backup, err := selectBackup(stanza, time.Unix(100, 0))
	if err != nil || backup.Label != "newest" {
		t.Fatalf("selected backup = %v, %v", backup, err)
	}
	if _, err := selectBackup(stanza, time.Unix(10, 0)); err == nil {
		t.Fatal("accepted target before all backups")
	}
}

func TestDecodeRestoreInfoRejectsUnusableRepository(t *testing.T) {
	valid := `[{"name":"alpha","status":{"code":0},"repo":[{"status":{"code":0}}]}]`
	if stanza, err := decodeRestoreInfo(valid, "alpha"); err != nil || stanza.Name != "alpha" {
		t.Fatalf("valid info = %v, %v", stanza, err)
	}
	for _, output := range []string{`invalid`, `[]`, `[{"name":"alpha","status":{"code":1}}]`, `[{"name":"alpha","status":{"code":0},"repo":[]}]`, `[{"name":"alpha","repo":[{"status":{"code":1}}]}]`} {
		if _, err := decodeRestoreInfo(output, "alpha"); err == nil {
			t.Fatalf("invalid info accepted: %s", output)
		}
	}
}

func TestRestoreCapacityIncludesRoundedHeadroomAndRejectsOverflow(t *testing.T) {
	for _, tt := range []struct{ original, restored, want uint64 }{{0, 0, 0}, {1, 0, 2}, {100, 200, 330}, {5, 6, 13}} {
		got, err := restoreCapacityRequired(tt.original, tt.restored)
		if err != nil || got != tt.want {
			t.Fatalf("capacity(%d,%d) = %d, %v, want %d", tt.original, tt.restored, got, err, tt.want)
		}
	}
	for _, tt := range [][2]uint64{{math.MaxUint64, 1}, {math.MaxUint64, 0}} {
		if _, err := restoreCapacityRequired(tt[0], tt[1]); err == nil {
			t.Fatal("capacity overflow accepted")
		}
	}
}

func TestRestoreHostCapacityParsers(t *testing.T) {
	if got, err := parseAvailableBytes("Filesystem 1024-blocks Used Available Capacity Mounted on\n/dev/test 100 40 60 40% /repo\n"); err != nil || got != 60*1024 {
		t.Fatalf("df = %d, %v", got, err)
	}
	if got, err := parseDirectorySizeBytes("12 /data\n"); err != nil || got != 12*1024 {
		t.Fatalf("du = %d, %v", got, err)
	}
	for _, output := range []string{"", "invalid", "-1 /data", "18446744073709551615 /data"} {
		if _, err := parseDirectorySizeBytes(output); err == nil {
			t.Fatalf("bad du accepted: %q", output)
		}
	}
	for _, output := range []string{"", "/dev/test 100 40 invalid 40% /repo", "/dev/test 100 40 18446744073709551615 40% /repo"} {
		if _, err := parseAvailableBytes(output); err == nil {
			t.Fatalf("bad df accepted: %q", output)
		}
	}
}
