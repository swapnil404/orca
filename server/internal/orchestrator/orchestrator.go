// Package orchestrator pushes full desired-state snapshots to connected agents.
package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"

	"github.com/swapnil404/orca/pkg/types"
	"github.com/swapnil404/orca/server/internal/store"
	"github.com/swapnil404/orca/server/internal/ws"
)

type desiredStateStore interface {
	ListCurrentDesiredStatesForHost(context.Context, string) ([]store.DesiredState, error)
	GetDesiredStateRevisionForHost(context.Context, string) (int64, error)
	ListActionableRestoreOperationsForHost(context.Context, string) ([]*types.RestoreOperation, error)
	GetRestoreRevisionForHost(context.Context, string) (int64, error)
}

type sessionHub interface {
	Get(string) (*ws.Session, bool)
}

// Orchestrator loads and pushes the current full desired state for a host.
type Orchestrator struct {
	store desiredStateStore
	hub   sessionHub

	mu        sync.Mutex
	hostLocks map[string]*sync.Mutex
}

// New creates a desired-state orchestrator.
func New(store desiredStateStore, hub sessionHub) *Orchestrator {
	return &Orchestrator{store: store, hub: hub, hostLocks: make(map[string]*sync.Mutex)}
}

// PushDesiredState sends one full current snapshot when hostID is connected.
// An offline host is not an error; registration will invoke this method again.
func (o *Orchestrator) PushDesiredState(ctx context.Context, hostID string) error {
	lock := o.hostLock(hostID)
	lock.Lock()
	defer lock.Unlock()

	session, connected := o.hub.Get(hostID)
	if !connected {
		return nil
	}

	// Read the revision first so a concurrent mutation can only make this value
	// lag the loaded state, never acknowledge state that was not sent.
	revision, err := o.store.GetDesiredStateRevisionForHost(ctx, hostID)
	if err != nil {
		return fmt.Errorf("load desired-state revision for host %s: %w", hostID, err)
	}
	states, err := o.store.ListCurrentDesiredStatesForHost(ctx, hostID)
	if err != nil {
		return fmt.Errorf("load desired state for host %s: %w", hostID, err)
	}
	restoreRevision, err := o.store.GetRestoreRevisionForHost(ctx, hostID)
	if err != nil {
		return fmt.Errorf("load restore revision for host %s: %w", hostID, err)
	}
	restoreOperations, err := o.store.ListActionableRestoreOperationsForHost(ctx, hostID)
	if err != nil {
		return fmt.Errorf("load restore operations for host %s: %w", hostID, err)
	}

	clusters := make([]*types.ClusterSpec, 0, len(states))
	for _, state := range states {
		var cluster types.ClusterSpec
		if err := json.Unmarshal(state.State, &cluster); err != nil {
			return fmt.Errorf("decode desired state for cluster %s: %w", state.ClusterID, err)
		}
		clusters = append(clusters, &cluster)
	}
	revisionValue := strconv.FormatInt(revision, 10)
	if restoreRevision > 0 {
		revisionValue = fmt.Sprintf("%d:%d", revision, restoreRevision)
	}
	message := &types.DesiredStateMessage{
		DesiredState: &types.DesiredState{Clusters: clusters, Revision: revisionValue, RestoreOperations: restoreOperations},
	}
	if err := session.SendDesiredState(message); err != nil {
		return fmt.Errorf("send desired state to host %s: %w", hostID, err)
	}
	return nil
}

func (o *Orchestrator) hostLock(hostID string) *sync.Mutex {
	o.mu.Lock()
	defer o.mu.Unlock()

	lock := o.hostLocks[hostID]
	if lock == nil {
		lock = &sync.Mutex{}
		o.hostLocks[hostID] = lock
	}
	return lock
}
