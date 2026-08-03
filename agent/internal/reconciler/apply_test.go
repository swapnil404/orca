package reconciler

import (
	"context"
	"errors"
	"reflect"
	"testing"

	orcadocker "github.com/swapnil404/orca/agent/internal/docker"
)

// Unimplemented Docker operations panic through the embedded interface, so an
// unexpected dependent action cannot silently succeed in these tests.
type applyDocker struct {
	DockerClient
	calls           []string
	stopErrors      map[string]error
	credentialError error
}

func (d *applyDocker) StopContainer(_ context.Context, id string) error {
	d.calls = append(d.calls, "stop:"+id)
	return d.stopErrors[id]
}
func (d *applyDocker) RemoveContainer(_ context.Context, id string) error {
	d.calls = append(d.calls, "remove:"+id)
	return nil
}
func (d *applyDocker) EnsureClusterPassword(_ context.Context, id string) (string, error) {
	d.calls = append(d.calls, "credentials:"+id)
	return "", d.credentialError
}
func (d *applyDocker) WriteConfig(context.Context, string, *orcadocker.ConfigMount) error {
	panic("unexpected configuration write")
}
func (d *applyDocker) ExecContainer(context.Context, string, []string) (string, error) {
	panic("unexpected container exec")
}
func (d *applyDocker) RemoveNetwork(_ context.Context, id string) error {
	d.calls = append(d.calls, "network:"+id)
	return nil
}
func (d *applyDocker) RemoveClusterData(_ context.Context, id string) error {
	d.calls = append(d.calls, "data:"+id)
	return nil
}
func (d *applyDocker) RemoveVolume(_ context.Context, id string) error {
	d.calls = append(d.calls, "volume:"+id)
	return nil
}

func assertStatuses(t *testing.T, results []ApplyResult, want ...ApplyStatus) {
	t.Helper()
	got := make([]ApplyStatus, len(results))
	for i, result := range results {
		got[i] = result.Status
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("statuses = %v, want %v", got, want)
	}
}

func TestApplyPrimaryFailureBlocksDependentsAndContinuesOtherClusters(t *testing.T) {
	failure := errors.New("credential storage unavailable")
	docker := &applyDocker{credentialError: failure}
	cluster, _ := settledCluster("alpha")
	actions := []Action{{Type: ActionCreatePrimary, ClusterID: "alpha", Spec: cluster}}
	for _, kind := range []ActionType{ActionCreateReplica, ActionCreatePgBouncer, ActionUpdateExtensions, ActionUpdatePgHba, ActionCreatePgBackRest, ActionRestartCluster} {
		actions = append(actions, Action{Type: kind, ClusterID: "alpha"})
	}
	actions = append(actions, Action{Type: ActionDeletePgBouncer, ClusterID: "beta", Spec: &ActualPgBouncer{ContainerId: "other-pool"}})
	results := apply(context.Background(), docker, nil, actions, &DesiredState{})
	assertStatuses(t, results, ApplyStatusFailed, ApplyStatusSkippedDependency, ApplyStatusSkippedDependency, ApplyStatusSkippedDependency, ApplyStatusSkippedDependency, ApplyStatusSkippedDependency, ApplyStatusSkippedDependency, ApplyStatusSuccess)
	if !errors.Is(results[0].Err, failure) {
		t.Fatalf("failure lost: %v", results[0].Err)
	}
	if !reflect.DeepEqual(docker.calls, []string{"credentials:alpha", "stop:other-pool", "remove:other-pool"}) {
		t.Fatalf("unexpected operations: %v", docker.calls)
	}
	for i := range actions {
		if results[i].Action.Type != actions[i].Type || results[i].Action.ClusterID != actions[i].ClusterID {
			t.Fatal("results no longer match action order")
		}
	}
}

func TestApplyReplicaDeleteFailureBlocksReplacementAndRetriesNextPass(t *testing.T) {
	failure := errors.New("container stop failed")
	docker := &applyDocker{stopErrors: map[string]error{"replica": failure}}
	replicaDelete := Action{Type: ActionDeleteReplica, ClusterID: "alpha", ReplicaID: "one", Spec: &ActualReplica{Id: "one", ContainerId: "replica"}}
	actions := []Action{replicaDelete, {Type: ActionCreateReplica, ClusterID: "alpha", ReplicaID: "one"}, {Type: ActionDeletePgBouncer, ClusterID: "beta", Spec: &ActualPgBouncer{ContainerId: "other-pool"}}}
	results := apply(context.Background(), docker, nil, actions, &DesiredState{})
	assertStatuses(t, results, ApplyStatusFailed, ApplyStatusSkippedDependency, ApplyStatusSuccess)
	if !errors.Is(results[0].Err, failure) {
		t.Fatal("stop error lost")
	}
	if !reflect.DeepEqual(docker.calls, []string{"stop:replica", "stop:other-pool", "remove:other-pool"}) {
		t.Fatalf("unsafe removal/replacement: %v", docker.calls)
	}
	delete(docker.stopErrors, "replica")
	docker.calls = nil
	assertStatuses(t, apply(context.Background(), docker, nil, []Action{replicaDelete}, &DesiredState{}), ApplyStatusSuccess)
	if !reflect.DeepEqual(docker.calls, []string{"stop:replica", "remove:replica"}) {
		t.Fatal("failure state leaked into next pass")
	}
}

func TestApplyDependentDeleteFailureProtectsPrimaryData(t *testing.T) {
	for _, kind := range []ActionType{ActionDeleteReplica, ActionDeletePgBouncer} {
		t.Run(string(kind), func(t *testing.T) {
			docker := &applyDocker{stopErrors: map[string]error{"dependent": errors.New("busy")}}
			var spec any = &ActualPgBouncer{ContainerId: "dependent"}
			if kind == ActionDeleteReplica {
				spec = &ActualReplica{Id: "one", ContainerId: "dependent"}
			}
			_, actual := settledCluster("alpha")
			results := apply(context.Background(), docker, nil, []Action{{Type: kind, ClusterID: "alpha", ReplicaID: "one", Spec: spec}, {Type: ActionDeletePrimary, ClusterID: "alpha", Spec: actual}}, &DesiredState{})
			assertStatuses(t, results, ApplyStatusFailed, ApplyStatusSkippedDependency)
			if !reflect.DeepEqual(docker.calls, []string{"stop:dependent"}) {
				t.Fatalf("primary/data touched despite dependent failure: %v", docker.calls)
			}
		})
	}
}

func TestApplyCompleteClusterDeletion(t *testing.T) {
	docker := &applyDocker{}
	_, actual := settledCluster("alpha")
	actual.Replicas = []*ActualReplica{{Id: "one", ContainerId: "replica"}}
	actual.PgBouncer = &ActualPgBouncer{ContainerId: "pool"}
	results := apply(context.Background(), docker, nil, Diff(&DesiredState{}, &ActualState{Clusters: []*ActualCluster{actual}}), &DesiredState{})
	assertStatuses(t, results, ApplyStatusSuccess, ApplyStatusSuccess, ApplyStatusSuccess)
	want := []string{"stop:replica", "remove:replica", "stop:pool", "remove:pool", "stop:alpha-primary", "remove:alpha-primary", "network:" + orcadocker.NetworkName("alpha"), "data:alpha", "volume:" + orcadocker.VolumeName("alpha")}
	if !reflect.DeepEqual(docker.calls, want) {
		t.Fatalf("deletion order = %v, want %v", docker.calls, want)
	}
}
