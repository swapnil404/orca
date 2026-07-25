package reconciler

import (
	"fmt"

	"github.com/swapnil404/orca/agent/internal/extensions"
	"github.com/swapnil404/orca/agent/internal/pgbouncer"
)

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
	// ActionUpdatePgBouncer replaces a PgBouncer container with updated configuration.
	ActionUpdatePgBouncer ActionType = "update_pgbouncer"
	// ActionDeletePgBouncer deletes a PgBouncer container.
	ActionDeletePgBouncer ActionType = "delete_pgbouncer"
	// ActionUpdateExtensions reconciles extensions on a running Postgres primary.
	ActionUpdateExtensions ActionType = "update_extensions"
)

// Action describes a single reconciliation operation.
type Action struct {
	Type      ActionType
	ClusterID string
	ReplicaID string // set for replica actions
	Spec      any    // the relevant spec needed to execute this action
}

type pgBouncerUpdateSpec struct {
	Desired *ClusterSpec
	Actual  *ActualPgBouncer
}

type extensionUpdateSpec struct {
	Desired []string
	Actual  *ActualCluster
	Actions []extensions.Action
}

// Diff computes the reconciliation actions required to make actual match desired.
func Diff(desired DesiredState, actual ActualState) []Action {
	actions := []Action{}
	actualClusters := make(map[string]*ActualCluster, len(actual.Clusters))
	for _, cluster := range actual.Clusters {
		actualClusters[cluster.Id] = cluster
	}

	for _, desiredCluster := range desired.Clusters {
		desiredPgBouncerConfig := ""
		pgBouncerConfigValid := true
		if desiredCluster.PgBouncer != nil {
			desiredPgBouncerConfig, pgBouncerConfigValid = generatedPgBouncerConfig(desiredCluster)
		}

		actualCluster, exists := actualClusters[desiredCluster.Id]
		if !exists {
			actions = append(actions, createClusterActions(desiredCluster)...)
			continue
		}

		if primaryNeedsUpdate(desiredCluster, actualCluster) {
			actions = append(actions, Action{
				Type:      ActionUpdatePrimary,
				ClusterID: desiredCluster.Id,
				Spec:      desiredCluster,
			})
		}

		actions = append(actions, diffReplicas(desiredCluster.Id, desiredCluster.Replicas, actualCluster.Replicas)...)
		pgBouncerActions := diffPgBouncer(desiredCluster, desiredPgBouncerConfig, pgBouncerConfigValid, actualCluster.PgBouncer)
		actions = append(actions, pgBouncerActions...)
		extensionActions := diffExtensions(desiredCluster, actualCluster)
		actions = append(actions, extensionActions...)
		delete(actualClusters, desiredCluster.Id)
	}

	for _, actualCluster := range actual.Clusters {
		if _, exists := actualClusters[actualCluster.Id]; exists {
			actions = append(actions, deleteClusterActions(actualCluster)...)
		}
	}

	return actions
}

func createClusterActions(cluster *ClusterSpec) []Action {
	actions := []Action{{
		Type:      ActionCreatePrimary,
		ClusterID: cluster.Id,
		Spec:      cluster,
	}}

	for _, replica := range cluster.Replicas {
		actions = append(actions, Action{
			Type:      ActionCreateReplica,
			ClusterID: cluster.Id,
			ReplicaID: replica.Id,
			Spec:      replica,
		})
	}

	if cluster.PgBouncer != nil {
		actions = append(actions, Action{
			Type:      ActionCreatePgBouncer,
			ClusterID: cluster.Id,
			Spec:      cluster,
		})
	}

	return actions
}

func deleteClusterActions(cluster *ActualCluster) []Action {
	actions := []Action{}

	for _, replica := range cluster.Replicas {
		actions = append(actions, Action{
			Type:      ActionDeleteReplica,
			ClusterID: cluster.Id,
			ReplicaID: replica.Id,
			Spec:      replica,
		})
	}

	if cluster.PgBouncer != nil {
		actions = append(actions, Action{
			Type:      ActionDeletePgBouncer,
			ClusterID: cluster.Id,
			Spec:      cluster.PgBouncer,
		})
	}

	actions = append(actions, Action{
		Type:      ActionDeletePrimary,
		ClusterID: cluster.Id,
		Spec:      cluster,
	})

	return actions
}

func primaryNeedsUpdate(desired *ClusterSpec, actual *ActualCluster) bool {
	return desired.Version != actual.Version || len(desired.Params) > 0
}

func diffReplicas(clusterID string, desired []*ReplicaSpec, actual []*ActualReplica) []Action {
	actions := []Action{}
	actualReplicas := make(map[string]*ActualReplica, len(actual))
	for _, replica := range actual {
		actualReplicas[replica.Id] = replica
	}

	for _, desiredReplica := range desired {
		actualReplica, exists := actualReplicas[desiredReplica.Id]
		if !exists {
			actions = append(actions, Action{
				Type:      ActionCreateReplica,
				ClusterID: clusterID,
				ReplicaID: desiredReplica.Id,
				Spec:      desiredReplica,
			})
			continue
		}
		if actualReplica.Status != "running" {
			actions = append(actions,
				Action{Type: ActionDeleteReplica, ClusterID: clusterID, ReplicaID: actualReplica.Id, Spec: actualReplica},
				Action{Type: ActionCreateReplica, ClusterID: clusterID, ReplicaID: desiredReplica.Id, Spec: desiredReplica},
			)
		}

		delete(actualReplicas, desiredReplica.Id)
	}

	for _, actualReplica := range actual {
		if _, exists := actualReplicas[actualReplica.Id]; exists {
			actions = append(actions, Action{
				Type:      ActionDeleteReplica,
				ClusterID: clusterID,
				ReplicaID: actualReplica.Id,
				Spec:      actualReplica,
			})
		}
	}

	return actions
}

func diffPgBouncer(desired *ClusterSpec, desiredConfig string, configValid bool, actual *ActualPgBouncer) []Action {
	if desired.PgBouncer != nil && actual == nil {
		return []Action{{
			Type:      ActionCreatePgBouncer,
			ClusterID: desired.Id,
			Spec:      desired,
		}}
	}
	if desired.PgBouncer == nil && actual != nil {
		return []Action{{
			Type:      ActionDeletePgBouncer,
			ClusterID: desired.Id,
			Spec:      actual,
		}}
	}
	if desired.PgBouncer == nil {
		return nil
	}
	if !configValid || desiredConfig != actual.Config || actual.Status != "running" {
		return []Action{{
			Type:      ActionUpdatePgBouncer,
			ClusterID: desired.Id,
			Spec: &pgBouncerUpdateSpec{
				Desired: desired,
				Actual:  actual,
			},
		}}
	}

	return nil
}

func diffExtensions(desired *ClusterSpec, actual *ActualCluster) []Action {
	if actual == nil || actual.ContainerId == "" || actual.Status != "running" || actual.EnabledExtensions == nil {
		return nil
	}
	extensionActions, err := extensions.Diff(desired.EnabledExtensions, actual.EnabledExtensions)
	if err != nil || len(extensionActions) == 0 {
		return nil
	}

	return []Action{{
		Type:      ActionUpdateExtensions,
		ClusterID: desired.Id,
		Spec: &extensionUpdateSpec{
			Desired: append([]string(nil), desired.EnabledExtensions...),
			Actual:  actual,
			Actions: extensionActions,
		},
	}}
}

func generatedPgBouncerConfig(desired *ClusterSpec) (string, bool) {
	config, err := pgbouncer.GeneratePgBouncerConfig(*desired)
	return config, err == nil
}

func extensionUpdate(action Action) (*extensionUpdateSpec, error) {
	update, ok := action.Spec.(*extensionUpdateSpec)
	if !ok || update.Actual == nil || update.Actual.ContainerId == "" {
		return nil, fmt.Errorf("%s action requires extension update state", action.Type)
	}
	return update, nil
}
