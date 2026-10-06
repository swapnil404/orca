package store

import (
	"database/sql"
	"errors"
	"reflect"
	"testing"
)

func TestRemoveSelectedReplicaPreservesOthers(t *testing.T) {
	original := []Replica{{ID: "first"}, {ID: "middle"}, {ID: "last"}}
	remaining, err := replicasWithout(original, "middle")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(remaining, []Replica{{ID: "first"}, {ID: "last"}}) {
		t.Fatalf("removed wrong replica: %+v", remaining)
	}
	if original[1].ID != "middle" {
		t.Fatal("changed original snapshot")
	}
}
func TestRemoveFinalReplicaProducesEmptyList(t *testing.T) {
	remaining, err := replicasWithout([]Replica{{ID: "only"}}, "only")
	if err != nil || remaining == nil || len(remaining) != 0 {
		t.Fatalf("remaining=%v err=%v", remaining, err)
	}
}
func TestRemoveMissingReplicaFails(t *testing.T) {
	_, err := replicasWithout([]Replica{{ID: "keep"}}, "missing")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected not found, got %v", err)
	}
}
