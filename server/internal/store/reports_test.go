package store

import (
	"testing"

	"github.com/swapnil404/orca/pkg/types"
)

func TestDecodeActualClusterLegacyVolumeField(t *testing.T) {
	for _, retired := range []string{``, `,"volumeExists":true`, `,"volume_exists":false`, `,"volumeExists":true,"volume_exists":false`} {
		t.Run(retired, func(t *testing.T) {
			payload := []byte(`{"id":"cluster","status":"running","postgresReady":true,"enabledExtensions":["pg_stat_statements"],"replicas":[{"id":"replica"}],"appliedRestartGeneration":"7"` + retired + `}`)
			actual := &types.ActualCluster{}
			if err := decodeActualCluster(payload, actual); err != nil {
				t.Fatal(err)
			}
			if actual.GetId() != "cluster" || actual.GetStatus() != "running" || !actual.GetPostgresReady() || actual.GetAppliedRestartGeneration() != 7 {
				t.Fatalf("lost current primary state: %v", actual)
			}
			if len(actual.GetEnabledExtensions()) != 1 || actual.GetEnabledExtensions()[0] != "pg_stat_statements" || len(actual.GetReplicas()) != 1 || actual.GetReplicas()[0].GetId() != "replica" {
				t.Fatalf("lost child resource state: %v", actual)
			}
		})
	}
}

func TestDecodeActualClusterRejectsInvalidReports(t *testing.T) {
	for _, payload := range []string{
		`{"id":"cluster","unexpectedField":true,"volumeExists":true}`,
		`{"id":"cluster","postgresReady":"wrong","volumeExists":true}`,
		`{"replicas":[{"unexpectedField":true}],"volumeExists":true}`,
		`{"id":`,
	} {
		t.Run(payload, func(t *testing.T) {
			if err := decodeActualCluster([]byte(payload), &types.ActualCluster{}); err == nil {
				t.Fatal("accepted invalid persisted report")
			}
		})
	}
}
