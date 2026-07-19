package reconciler

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
