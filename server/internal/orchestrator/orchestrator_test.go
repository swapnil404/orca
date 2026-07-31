package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/swapnil404/orca/pkg/types"
	storetypes "github.com/swapnil404/orca/server/internal/store"
	"github.com/swapnil404/orca/server/internal/ws"
	"google.golang.org/protobuf/proto"
)

type snapshotStore struct {
	states          map[string][]storetypes.DesiredState
	revision        int64
	restoreRevision int64
	restores        []*types.RestoreOperation
	calls           []string
	fail            error
}

func (s *snapshotStore) GetDesiredStateRevisionForHost(_ context.Context, host string) (int64, error) {
	s.calls = append(s.calls, "revision:"+host)
	return s.revision, s.fail
}
func (s *snapshotStore) ListCurrentDesiredStatesForHost(_ context.Context, host string) ([]storetypes.DesiredState, error) {
	s.calls = append(s.calls, "states:"+host)
	return s.states[host], s.fail
}
func (s *snapshotStore) GetRestoreRevisionForHost(_ context.Context, host string) (int64, error) {
	s.calls = append(s.calls, "restore-revision:"+host)
	return s.restoreRevision, s.fail
}
func (s *snapshotStore) ListActionableRestoreOperationsForHost(_ context.Context, host string) ([]*types.RestoreOperation, error) {
	s.calls = append(s.calls, "restores:"+host)
	return s.restores, s.fail
}

type snapshotConnection struct {
	frames [][]byte
	kinds  []int
}

func (*snapshotConnection) Close() error                     { return nil }
func (*snapshotConnection) SetWriteDeadline(time.Time) error { return nil }
func (c *snapshotConnection) WriteMessage(kind int, data []byte) error {
	c.kinds = append(c.kinds, kind)
	c.frames = append(c.frames, append([]byte(nil), data...))
	return nil
}

func storedCluster(t *testing.T, id, version string) storetypes.DesiredState {
	t.Helper()
	data, err := json.Marshal(&types.ClusterSpec{Id: id, Version: version})
	if err != nil {
		t.Fatal(err)
	}
	return storetypes.DesiredState{ClusterID: id, State: data}
}
func readSnapshot(t *testing.T, c *snapshotConnection) *types.DesiredState {
	t.Helper()
	if len(c.frames) != 1 || c.kinds[0] != websocket.BinaryMessage {
		t.Fatalf("expected one binary snapshot, got %d frames", len(c.frames))
	}
	var message types.DesiredStateMessage
	if err := proto.Unmarshal(c.frames[0], &message); err != nil {
		t.Fatal(err)
	}
	return message.DesiredState
}

func TestReconnectSendsLatestFullSnapshotToCorrectHost(t *testing.T) {
	store := &snapshotStore{states: map[string][]storetypes.DesiredState{"alpha": {storedCluster(t, "old", "17")}}, revision: 1}
	hub := ws.NewHub()
	orchestrator := New(store, hub)
	if err := orchestrator.PushDesiredState(context.Background(), "alpha"); err != nil {
		t.Fatal(err)
	}
	if len(store.calls) != 0 {
		t.Fatal("offline push accessed store")
	}
	store.states["alpha"] = []storetypes.DesiredState{storedCluster(t, "keep", "17"), storedCluster(t, "new", "18")}
	store.revision = 2
	if err := orchestrator.PushDesiredState(context.Background(), "alpha"); err != nil {
		t.Fatal(err)
	}
	store.states["alpha"] = []storetypes.DesiredState{storedCluster(t, "keep", "18")}
	store.revision = 3
	store.restoreRevision = 4
	store.restores = []*types.RestoreOperation{{Id: "restore", Intent: "preflight", SourceClusterId: "keep"}}
	alpha, beta := &snapshotConnection{}, &snapshotConnection{}
	alphaSession, betaSession := ws.NewSession(alpha), ws.NewSession(beta)
	defer alphaSession.Close()
	defer betaSession.Close()
	hub.Register("alpha", alphaSession)
	hub.Register("beta", betaSession)
	if err := orchestrator.PushDesiredState(context.Background(), "alpha"); err != nil {
		t.Fatal(err)
	}
	got := readSnapshot(t, alpha)
	want := &types.DesiredState{Revision: "3:4", Clusters: []*types.ClusterSpec{{Id: "keep", Version: "18"}}, RestoreOperations: store.restores}
	if !proto.Equal(got, want) {
		t.Fatalf("snapshot = %v, want %v", got, want)
	}
	if len(beta.frames) != 0 {
		t.Fatal("snapshot crossed host boundary")
	}
	if len(store.calls) != 4 || store.calls[0] != "revision:alpha" {
		t.Fatalf("revision must precede state read: %v", store.calls)
	}
}

func TestEmptySnapshotClearsRemovedClusters(t *testing.T) {
	store := &snapshotStore{revision: 8}
	hub := ws.NewHub()
	connection := &snapshotConnection{}
	session := ws.NewSession(connection)
	defer session.Close()
	hub.Register("alpha", session)
	if err := New(store, hub).PushDesiredState(context.Background(), "alpha"); err != nil {
		t.Fatal(err)
	}
	got := readSnapshot(t, connection)
	if got.Revision != "8" || len(got.Clusters) != 0 {
		t.Fatalf("empty full snapshot = %v", got)
	}
}

func TestSnapshotErrorsDoNotSendPartialState(t *testing.T) {
	for _, name := range []string{"store failure", "invalid state"} {
		t.Run(name, func(t *testing.T) {
			store := &snapshotStore{states: map[string][]storetypes.DesiredState{"alpha": {{ClusterID: "bad", State: json.RawMessage("{invalid")}}}}
			if name == "store failure" {
				store.fail = errors.New("store unavailable")
			}
			connection := &snapshotConnection{}
			hub := ws.NewHub()
			session := ws.NewSession(connection)
			defer session.Close()
			hub.Register("alpha", session)
			err := New(store, hub).PushDesiredState(context.Background(), "alpha")
			if err == nil {
				t.Fatal("failure was hidden")
			}
			if store.fail != nil && !errors.Is(err, store.fail) {
				t.Fatal("store error not preserved")
			}
			if len(connection.frames) != 0 {
				t.Fatal("partial snapshot sent")
			}
		})
	}
}
