package api

import (
	"net/http/httptest"
	"testing"

	"github.com/swapnil404/orca/server/internal/store"
)

func TestValidateClusterRequestPgBackRest(t *testing.T) {
	tests := []struct {
		name   string
		config *store.PgBackRestConfig
		wantOK bool
	}{
		{
			name: "valid",
			config: &store.PgBackRestConfig{
				RepoPath: "/repo", RetentionFull: 2, RetentionDiff: 4, FullIntervalSeconds: 86400,
			},
			wantOK: true,
		},
		{
			name:   "missing repository",
			config: &store.PgBackRestConfig{RetentionFull: 2, RetentionDiff: 4},
		},
		{
			name:   "invalid retention",
			config: &store.PgBackRestConfig{RepoPath: "/repo", RetentionFull: 0, RetentionDiff: 4},
		},
		{
			name:   "negative interval",
			config: &store.PgBackRestConfig{RepoPath: "/repo", RetentionFull: 2, RetentionDiff: 4, IncrIntervalSeconds: -1},
		},
		{
			name:   "repository newline",
			config: &store.PgBackRestConfig{RepoPath: "/repo\n[other]", RetentionFull: 2, RetentionDiff: 4},
		},
		{
			name:   "interval too large",
			config: &store.PgBackRestConfig{RepoPath: "/repo", RetentionFull: 2, RetentionDiff: 4, FullIntervalSeconds: maxBackupIntervalSeconds + 1},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			ok := validateClusterRequest(response, clusterRequest{
				HostID: "host", Name: "orders", PostgresVersion: "17", PgBackRest: test.config,
			}, true)
			if ok != test.wantOK {
				t.Fatalf("validateClusterRequest() = %t, want %t; response = %s", ok, test.wantOK, response.Body.String())
			}
		})
	}
}
