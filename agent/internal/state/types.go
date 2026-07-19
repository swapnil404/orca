package state

// DesiredState is the desired set of clusters managed by the agent.
type DesiredState struct {
	ClusterID string
	Clusters  []ClusterSpec
}

// ClusterSpec describes a desired Postgres cluster.
type ClusterSpec struct {
	ID        string
	Version   string            // e.g. "16"
	Params    map[string]string // postgresql.conf overrides
	Replicas  []ReplicaSpec
	PgBouncer *PgBouncerSpec // nil if not wanted
}

// ReplicaSpec describes a desired Postgres replica.
type ReplicaSpec struct {
	ID string
}

// PgBouncerSpec describes a desired PgBouncer sidecar.
type PgBouncerSpec struct {
	PoolMode string
}
