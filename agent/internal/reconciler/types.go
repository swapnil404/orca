package reconciler

import (
	"github.com/swapnil404/orca/agent/internal/state"
	orcatypes "github.com/swapnil404/orca/pkg/types"
)

// DesiredState is the desired set of clusters managed by the agent.
type DesiredState = state.DesiredState

// ClusterSpec describes a desired Postgres cluster.
type ClusterSpec = state.ClusterSpec

// ReplicaSpec describes a desired Postgres replica.
type ReplicaSpec = state.ReplicaSpec

// PgBouncerSpec describes a desired PgBouncer sidecar.
type PgBouncerSpec = state.PgBouncerSpec

// ActualState is the set of clusters currently observed by the agent.
type ActualState = orcatypes.ActualState

// ActualCluster describes an observed Postgres primary and its child resources.
type ActualCluster = orcatypes.ActualCluster

// ActualReplica describes an observed Postgres replica.
type ActualReplica = orcatypes.ActualReplica

// ActualPgBouncer describes an observed PgBouncer sidecar.
type ActualPgBouncer = orcatypes.ActualPgBouncer
