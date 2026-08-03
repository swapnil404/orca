package ws

import (
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/swapnil404/orca/pkg/types"
	"google.golang.org/protobuf/proto"
)

type sessionConnection struct {
	writes        atomic.Int32
	active        atomic.Int32
	overlapping   atomic.Bool
	closes        atomic.Int32
	deadlines     []time.Time
	frames        [][]byte
	kinds         []int
	deadlineError error
	writeError    error
	controlError  error
}

func (c *sessionConnection) Close() error { c.closes.Add(1); return nil }
func (c *sessionConnection) SetWriteDeadline(deadline time.Time) error {
	c.deadlines = append(c.deadlines, deadline)
	return c.deadlineError
}
func (c *sessionConnection) WriteMessage(kind int, data []byte) error {
	if c.active.Add(1) != 1 {
		c.overlapping.Store(true)
	}
	defer c.active.Add(-1)
	runtime.Gosched()
	c.writes.Add(1)
	c.kinds = append(c.kinds, kind)
	c.frames = append(c.frames, append([]byte(nil), data...))
	return c.writeError
}
func (c *sessionConnection) WriteControl(int, []byte, time.Time) error { return c.controlError }

func TestSessionWritesBinarySnapshotAndResetsDeadline(t *testing.T) {
	connection := &sessionConnection{}
	session := NewSession(connection)
	defer session.Close()
	want := &types.DesiredStateMessage{DesiredState: &types.DesiredState{Revision: "latest", Clusters: []*types.ClusterSpec{{Id: "alpha", Version: "17"}}}}
	if err := session.SendDesiredState(want); err != nil {
		t.Fatal(err)
	}
	if len(connection.frames) != 1 || connection.kinds[0] != websocket.BinaryMessage {
		t.Fatal("expected one binary snapshot")
	}
	var got types.DesiredStateMessage
	if err := proto.Unmarshal(connection.frames[0], &got); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(&got, want) {
		t.Fatalf("snapshot changed: %v", &got)
	}
	if len(connection.deadlines) != 2 || connection.deadlines[0].IsZero() || !connection.deadlines[1].IsZero() {
		t.Fatalf("deadline lifecycle = %v", connection.deadlines)
	}
}

func TestSessionConcurrentSnapshotWritesAreSerialized(t *testing.T) {
	connection := &sessionConnection{}
	session := NewSession(connection)
	defer session.Close()
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := session.SendDesiredState(&types.DesiredStateMessage{DesiredState: &types.DesiredState{Revision: "current"}}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if connection.overlapping.Load() || connection.writes.Load() != 32 {
		t.Fatalf("overlap = %v, writes = %d", connection.overlapping.Load(), connection.writes.Load())
	}
}

func TestSessionTransportFailureClosesOnce(t *testing.T) {
	failure := errors.New("transport unavailable")
	for _, kind := range []string{"deadline", "write", "ping"} {
		t.Run(kind, func(t *testing.T) {
			connection := &sessionConnection{}
			session := NewSession(connection)
			var err error
			switch kind {
			case "deadline":
				connection.deadlineError = failure
				err = session.SendDesiredState(&types.DesiredStateMessage{})
			case "write":
				connection.writeError = failure
				err = session.SendDesiredState(&types.DesiredStateMessage{})
			case "ping":
				connection.controlError = failure
				err = session.Ping()
			}
			if !errors.Is(err, failure) {
				t.Fatalf("transport error lost: %v", err)
			}
			select {
			case <-session.Done():
			default:
				t.Fatal("failed transport remained open")
			}
			var wg sync.WaitGroup
			for i := 0; i < 16; i++ {
				wg.Add(1)
				go func() { defer wg.Done(); session.Close() }()
			}
			wg.Wait()
			if connection.closes.Load() != 1 {
				t.Fatalf("close calls = %d", connection.closes.Load())
			}
		})
	}
}

func TestSessionWithoutWriterReturnsError(t *testing.T) {
	session := NewSession()
	defer session.Close()
	if !errors.Is(session.SendDesiredState(&types.DesiredStateMessage{}), errSessionCannotWrite) {
		t.Fatal("missing writer error lost")
	}
	if !errors.Is(session.Ping(), errSessionCannotWrite) {
		t.Fatal("missing control writer error lost")
	}
}
