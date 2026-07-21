package reconciler

import "testing"

func TestDesiredReplica(t *testing.T) {
	desired := DesiredState{Clusters: []*ClusterSpec{{
		Id:       "cluster",
		Replicas: []*ReplicaSpec{{Id: "first"}, {Id: "second"}},
	}}}

	t.Run("returns one-based slot index", func(t *testing.T) {
		cluster, index, err := desiredReplica(desired, "cluster", "second")
		if err != nil {
			t.Fatalf("desiredReplica returned error: %v", err)
		}
		if cluster.Id != "cluster" || index != 2 {
			t.Fatalf("got cluster %q index %d, want cluster index 2", cluster.Id, index)
		}
	})

	t.Run("rejects unknown replica", func(t *testing.T) {
		if _, _, err := desiredReplica(desired, "cluster", "missing"); err == nil {
			t.Fatal("desiredReplica returned no error")
		}
	})
}
