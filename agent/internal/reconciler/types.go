package reconciler

import "github.com/betterorca/betterorca/agent/internal/state"

// DesiredState is the desired set of clusters managed by the agent.
type DesiredState = state.DesiredState

// ClusterSpec describes a desired Postgres cluster.
type ClusterSpec = state.ClusterSpec

// ReplicaSpec describes a desired Postgres replica.
type ReplicaSpec = state.ReplicaSpec

// PgBouncerSpec describes a desired PgBouncer sidecar.
type PgBouncerSpec = state.PgBouncerSpec

// ActualState is the set of clusters currently observed by the agent.
type ActualState struct {
	Clusters []ActualCluster
}

// ActualCluster describes an observed Postgres primary and its child resources.
type ActualCluster struct {
	ID          string
	ContainerID string
	Status      string // running, stopped, unknown
	Version     string
	Replicas    []ActualReplica
	PgBouncer   *ActualPgBouncer
}

// ActualReplica describes an observed Postgres replica.
type ActualReplica struct {
	ID          string
	ContainerID string
	Status      string
}

// ActualPgBouncer describes an observed PgBouncer sidecar.
type ActualPgBouncer struct {
	ContainerID string
	Status      string
}
