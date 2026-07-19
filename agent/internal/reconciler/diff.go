package reconciler

// ActionType identifies the kind of reconciliation action to execute.
type ActionType string

const (
	// ActionCreatePrimary creates a Postgres primary container.
	ActionCreatePrimary ActionType = "create_primary"
	// ActionUpdatePrimary updates a Postgres primary container.
	ActionUpdatePrimary ActionType = "update_primary"
	// ActionDeletePrimary deletes a Postgres primary container.
	ActionDeletePrimary ActionType = "delete_primary"
	// ActionCreateReplica creates a Postgres replica container.
	ActionCreateReplica ActionType = "create_replica"
	// ActionDeleteReplica deletes a Postgres replica container.
	ActionDeleteReplica ActionType = "delete_replica"
	// ActionCreatePgBouncer creates a PgBouncer container.
	ActionCreatePgBouncer ActionType = "create_pgbouncer"
	// ActionDeletePgBouncer deletes a PgBouncer container.
	ActionDeletePgBouncer ActionType = "delete_pgbouncer"
)

// Action describes a single reconciliation operation.
type Action struct {
	Type      ActionType
	ClusterID string
	ReplicaID string // set for replica actions
	Spec      any    // the relevant spec needed to execute this action
}

// Diff computes the reconciliation actions required to make actual match desired.
func Diff(desired DesiredState, actual ActualState) []Action {
	actions := []Action{}
	actualClusters := make(map[string]ActualCluster, len(actual.Clusters))
	for _, cluster := range actual.Clusters {
		actualClusters[cluster.ID] = cluster
	}

	for _, desiredCluster := range desired.Clusters {
		actualCluster, exists := actualClusters[desiredCluster.ID]
		if !exists {
			actions = append(actions, createClusterActions(desiredCluster)...)
			continue
		}

		if primaryNeedsUpdate(desiredCluster, actualCluster) {
			actions = append(actions, Action{
				Type:      ActionUpdatePrimary,
				ClusterID: desiredCluster.ID,
				Spec:      desiredCluster,
			})
		}

		actions = append(actions, diffReplicas(desiredCluster.ID, desiredCluster.Replicas, actualCluster.Replicas)...)
		actions = append(actions, diffPgBouncer(desiredCluster.ID, desiredCluster.PgBouncer, actualCluster.PgBouncer)...)
		delete(actualClusters, desiredCluster.ID)
	}

	for _, actualCluster := range actual.Clusters {
		if _, exists := actualClusters[actualCluster.ID]; exists {
			actions = append(actions, deleteClusterActions(actualCluster)...)
		}
	}

	return actions
}

func createClusterActions(cluster ClusterSpec) []Action {
	actions := []Action{{
		Type:      ActionCreatePrimary,
		ClusterID: cluster.ID,
		Spec:      cluster,
	}}

	for _, replica := range cluster.Replicas {
		actions = append(actions, Action{
			Type:      ActionCreateReplica,
			ClusterID: cluster.ID,
			ReplicaID: replica.ID,
			Spec:      replica,
		})
	}

	if cluster.PgBouncer != nil {
		actions = append(actions, Action{
			Type:      ActionCreatePgBouncer,
			ClusterID: cluster.ID,
			Spec:      *cluster.PgBouncer,
		})
	}

	return actions
}

func deleteClusterActions(cluster ActualCluster) []Action {
	actions := []Action{}

	for _, replica := range cluster.Replicas {
		actions = append(actions, Action{
			Type:      ActionDeleteReplica,
			ClusterID: cluster.ID,
			ReplicaID: replica.ID,
			Spec:      replica,
		})
	}

	if cluster.PgBouncer != nil {
		actions = append(actions, Action{
			Type:      ActionDeletePgBouncer,
			ClusterID: cluster.ID,
			Spec:      *cluster.PgBouncer,
		})
	}

	actions = append(actions, Action{
		Type:      ActionDeletePrimary,
		ClusterID: cluster.ID,
		Spec:      cluster,
	})

	return actions
}

func primaryNeedsUpdate(desired ClusterSpec, actual ActualCluster) bool {
	return desired.Version != actual.Version || len(desired.Params) > 0
}

func diffReplicas(clusterID string, desired []ReplicaSpec, actual []ActualReplica) []Action {
	actions := []Action{}
	actualReplicas := make(map[string]ActualReplica, len(actual))
	for _, replica := range actual {
		actualReplicas[replica.ID] = replica
	}

	for _, desiredReplica := range desired {
		if _, exists := actualReplicas[desiredReplica.ID]; !exists {
			actions = append(actions, Action{
				Type:      ActionCreateReplica,
				ClusterID: clusterID,
				ReplicaID: desiredReplica.ID,
				Spec:      desiredReplica,
			})
			continue
		}

		delete(actualReplicas, desiredReplica.ID)
	}

	for _, actualReplica := range actual {
		if _, exists := actualReplicas[actualReplica.ID]; exists {
			actions = append(actions, Action{
				Type:      ActionDeleteReplica,
				ClusterID: clusterID,
				ReplicaID: actualReplica.ID,
				Spec:      actualReplica,
			})
		}
	}

	return actions
}

func diffPgBouncer(clusterID string, desired *PgBouncerSpec, actual *ActualPgBouncer) []Action {
	if desired != nil && actual == nil {
		return []Action{{
			Type:      ActionCreatePgBouncer,
			ClusterID: clusterID,
			Spec:      *desired,
		}}
	}
	if desired == nil && actual != nil {
		return []Action{{
			Type:      ActionDeletePgBouncer,
			ClusterID: clusterID,
			Spec:      *actual,
		}}
	}

	return nil
}
