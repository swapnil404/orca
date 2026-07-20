package state

import orcatypes "github.com/betterorca/betterorca/pkg/types"

// DesiredState is the desired set of clusters managed by the agent.
type DesiredState = orcatypes.DesiredState

// ClusterSpec describes a desired Postgres cluster.
type ClusterSpec = orcatypes.ClusterSpec

// ReplicaSpec describes a desired Postgres replica.
type ReplicaSpec = orcatypes.ReplicaSpec

// PgBouncerSpec describes a desired PgBouncer sidecar.
type PgBouncerSpec = orcatypes.PgBouncerSpec
