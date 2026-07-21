// Package orchestrator pushes full desired-state snapshots to connected agents.
package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/swapnil404/orca/pkg/types"
	"github.com/swapnil404/orca/server/internal/store"
	"github.com/swapnil404/orca/server/internal/ws"
)

type desiredStateStore interface {
	ListCurrentDesiredStatesForHost(context.Context, string) ([]store.DesiredState, error)
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

	states, err := o.store.ListCurrentDesiredStatesForHost(ctx, hostID)
	if err != nil {
		return fmt.Errorf("load desired state for host %s: %w", hostID, err)
	}

	clusters := make([]*types.ClusterSpec, 0, len(states))
	for _, state := range states {
		var cluster types.ClusterSpec
		if err := json.Unmarshal(state.State, &cluster); err != nil {
			return fmt.Errorf("decode desired state for cluster %s: %w", state.ClusterID, err)
		}
		clusters = append(clusters, &cluster)
	}

	message := &types.DesiredStateMessage{
		DesiredState: &types.DesiredState{Clusters: clusters},
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
