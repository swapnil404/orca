package reconciler

import (
	"reflect"
	"testing"

	"github.com/swapnil404/orca/agent/internal/pgbouncer"
	"github.com/swapnil404/orca/pkg/types"
	"google.golang.org/protobuf/proto"
)

func settledCluster(id string) (*ClusterSpec, *ActualCluster) {
	return &ClusterSpec{Id: id, Version: "17"}, &ActualCluster{Id: id, Version: "17", ContainerId: id + "-primary", Status: "running", NetworkName: "orca-" + id + "-network"}
}

func actionTypes(actions []Action) []ActionType {
	result := make([]ActionType, len(actions))
	for i, action := range actions {
		result[i] = action.Type
	}
	return result
}

func assertActionTypes(t *testing.T, actions []Action, want ...ActionType) {
	t.Helper()
	got := actionTypes(actions)
	if len(got) != len(want) || len(got) > 0 && !reflect.DeepEqual(got, want) {
		t.Fatalf("action order = %v, want %v", got, want)
	}
}

func TestDiffClusterLifecycle(t *testing.T) {
	desired, actual := settledCluster("alpha")
	tests := []struct {
		name    string
		desired *DesiredState
		actual  *ActualState
		want    []ActionType
	}{
		{"empty", &DesiredState{}, &ActualState{}, nil},
		{"full resync", &DesiredState{Clusters: []*ClusterSpec{desired}}, &ActualState{}, []ActionType{ActionCreatePrimary}},
		{"unchanged", &DesiredState{Clusters: []*ClusterSpec{desired}}, &ActualState{Clusters: []*ActualCluster{actual}}, nil},
		{"removed", &DesiredState{}, &ActualState{Clusters: []*ActualCluster{actual}}, []ActionType{ActionDeletePrimary}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			beforeDesired := proto.Clone(tt.desired)
			beforeActual := proto.Clone(tt.actual)
			assertActionTypes(t, Diff(tt.desired, tt.actual), tt.want...)
			if !proto.Equal(beforeDesired, tt.desired) || !proto.Equal(beforeActual, tt.actual) {
				t.Fatal("Diff mutated its input")
			}
		})
	}
}

func TestDiffCreatesDependenciesAfterPrimary(t *testing.T) {
	desired, _ := settledCluster("alpha")
	desired.PgHba = &types.PgHbaSpec{}
	desired.Replicas = []*ReplicaSpec{{Id: "one"}, {Id: "two"}}
	desired.PgBouncer = &PgBouncerSpec{}
	desired.PgBackRest = &types.PgBackRestSpec{}
	actions := Diff(&DesiredState{Clusters: []*ClusterSpec{desired}}, &ActualState{})
	assertActionTypes(t, actions, ActionCreatePrimary, ActionUpdatePgHba, ActionCreateReplica, ActionCreateReplica, ActionCreatePgBouncer, ActionCreatePgBackRest)
	if actions[2].ReplicaID != "one" || actions[3].ReplicaID != "two" {
		t.Fatal("replica identities were lost")
	}
}

func TestDiffDeletesDependenciesBeforePrimary(t *testing.T) {
	_, actual := settledCluster("alpha")
	actual.Replicas = []*ActualReplica{{Id: "one", ContainerId: "replica"}}
	actual.PgBouncer = &ActualPgBouncer{ContainerId: "pool"}
	actual.Backup = &ActualBackup{}
	actions := Diff(&DesiredState{}, &ActualState{Clusters: []*ActualCluster{actual}})
	assertActionTypes(t, actions, ActionDeleteReplica, ActionDeletePgBouncer, ActionDeletePgBackRest, ActionDeletePrimary)
	backup, ok := actions[2].Spec.(*pgBackRestDeleteSpec)
	if !ok || !backup.DeleteCluster {
		t.Fatal("cluster removal must remove backup data")
	}
}

func TestDiffReplacesPrimaryAfterRemovingDependents(t *testing.T) {
	desired, actual := settledCluster("alpha")
	desired.Version = "18"
	desired.Replicas = []*ReplicaSpec{{Id: "one"}}
	actual.Replicas = []*ActualReplica{{Id: "one", ContainerId: "replica", Status: "running", NetworkName: actual.NetworkName}}
	desired.PgBouncer = &PgBouncerSpec{PoolMode: "transaction", MaxConnections: 50, PublishAddress: "127.0.0.1", PublishPort: 6432}
	actual.PgBouncer = &ActualPgBouncer{ContainerId: "pool", Status: "running", NetworkName: actual.NetworkName}
	assertActionTypes(t, Diff(&DesiredState{Clusters: []*ClusterSpec{desired}}, &ActualState{Clusters: []*ActualCluster{actual}}), ActionDeleteReplica, ActionDeletePgBouncer, ActionUpdatePrimary, ActionCreateReplica, ActionCreatePgBouncer)
}

func TestDiffReplicasUsesIdentityRatherThanPosition(t *testing.T) {
	desired, actual := settledCluster("alpha")
	desired.Replicas = []*ReplicaSpec{{Id: "keep"}, {Id: "new"}}
	actual.Replicas = []*ActualReplica{{Id: "old", ContainerId: "old-container", Status: "running", NetworkName: actual.NetworkName}, {Id: "keep", ContainerId: "keep-container", Status: "running", NetworkName: actual.NetworkName}}
	actions := Diff(&DesiredState{Clusters: []*ClusterSpec{desired}}, &ActualState{Clusters: []*ActualCluster{actual}})
	assertActionTypes(t, actions, ActionCreateReplica, ActionDeleteReplica)
	if actions[0].ReplicaID != "new" || actions[1].ReplicaID != "old" {
		t.Fatalf("wrong replica targets: %+v", actions)
	}
}

func TestDiffRecoversStoppedPrimary(t *testing.T) {
	desired, actual := settledCluster("alpha")
	actual.Status = "exited"
	assertActionTypes(t, Diff(&DesiredState{Clusters: []*ClusterSpec{desired}}, &ActualState{Clusters: []*ActualCluster{actual}}), ActionRecoverPrimary)
}

func TestDiffRestartGeneration(t *testing.T) {
	for _, tt := range []struct {
		name             string
		desired, applied uint64
		want             []ActionType
	}{
		{"new request", 2, 1, []ActionType{ActionRestartCluster}}, {"already applied", 2, 2, nil}, {"stale snapshot", 1, 2, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			desired, actual := settledCluster("alpha")
			desired.RestartGeneration = tt.desired
			actual.AppliedRestartGeneration = tt.applied
			assertActionTypes(t, Diff(&DesiredState{Clusters: []*ClusterSpec{desired}}, &ActualState{Clusters: []*ActualCluster{actual}}), tt.want...)
		})
	}
}

func TestDiffPoolConfigurationAndPublishedEndpoint(t *testing.T) {
	for _, change := range []string{"none", "port", "address", "config", "stopped"} {
		t.Run(change, func(t *testing.T) {
			desired, actual := settledCluster("alpha")
			desired.PgBouncer = &PgBouncerSpec{PoolMode: "transaction", MaxConnections: 50, PublishAddress: "127.0.0.1", PublishPort: 6432}
			config, err := pgbouncer.GeneratePgBouncerConfig(desired)
			if err != nil {
				t.Fatal(err)
			}
			actual.PgBouncer = &ActualPgBouncer{ContainerId: "pool", Status: "running", Config: config, NetworkName: actual.NetworkName, PublishedAddress: "127.0.0.1", PublishedPort: 6432}
			switch change {
			case "port":
				desired.PgBouncer.PublishPort++
			case "address":
				desired.PgBouncer.PublishAddress = "0.0.0.0"
			case "config":
				desired.PgBouncer.MaxConnections++
			case "stopped":
				actual.PgBouncer.Status = "exited"
			}
			actions := Diff(&DesiredState{Clusters: []*ClusterSpec{desired}}, &ActualState{Clusters: []*ActualCluster{actual}})
			if change == "none" {
				assertActionTypes(t, actions)
			} else {
				assertActionTypes(t, actions, ActionUpdatePgBouncer)
			}
		})
	}
}
