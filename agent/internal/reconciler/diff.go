package reconciler

import (
	"fmt"
	"strings"

	"github.com/swapnil404/orca/agent/internal/extensions"
	"github.com/swapnil404/orca/agent/internal/pgbackrest"
	"github.com/swapnil404/orca/agent/internal/pgbouncer"
	"github.com/swapnil404/orca/agent/internal/postgres"
)

// ActionType identifies the kind of reconciliation action to execute.
type ActionType string

const (
	// ActionCreatePrimary creates a Postgres primary container.
	ActionCreatePrimary ActionType = "create_primary"
	// ActionUpdatePrimary updates a Postgres primary container.
	ActionUpdatePrimary ActionType = "update_primary"
	// ActionRecoverPrimary starts a desired Postgres primary that is stopped.
	ActionRecoverPrimary ActionType = "recover_primary"
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
	// ActionUpdatePgHba reconciles client authentication on all PostgreSQL nodes.
	ActionUpdatePgHba ActionType = "update_pg_hba"
	// ActionObservePgHba reports a failure reading the active authentication configuration.
	ActionObservePgHba ActionType = "observe_pg_hba"
	// ActionCreatePgBackRest configures a new pgBackRest stanza and schedule.
	ActionCreatePgBackRest ActionType = "create_pgbackrest"
	// ActionUpdatePgBackRest updates an existing pgBackRest stanza and schedule.
	ActionUpdatePgBackRest ActionType = "update_pgbackrest"
	// ActionDeletePgBackRest disables pgBackRest for a retained primary.
	ActionDeletePgBackRest ActionType = "delete_pgbackrest"
	// ActionRunPgBackRestBackup reports an asynchronous scheduled backup attempt.
	ActionRunPgBackRestBackup ActionType = "run_pgbackrest_backup"
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

type primaryUpdateSpec struct {
	Desired *ClusterSpec
	Actual  *ActualCluster
}

type pgBackRestDeleteSpec struct {
	Backup        *ActualBackup
	DeleteCluster bool
}

type extensionUpdateSpec struct {
	Desired []string
	Actual  *ActualCluster
	Actions []extensions.Action
	DiffErr error `json:"-"`
}

type pgHbaUpdateSpec struct {
	Desired *ClusterSpec
	Actual  *ActualCluster
}

// Diff computes the reconciliation actions required to make actual match desired.
func Diff(desired *DesiredState, actual *ActualState) []Action {
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

		primaryReplacement := primaryRequiresReplacement(desiredCluster, actualCluster)
		primaryWillChange := actualCluster.ContainerId == "" || primaryNeedsUpdate(desiredCluster, actualCluster)
		if actualCluster.ContainerId == "" {
			actions = append(actions, Action{Type: ActionCreatePrimary, ClusterID: desiredCluster.Id, Spec: desiredCluster})
		} else if primaryNeedsUpdate(desiredCluster, actualCluster) {
			actions = append(actions, Action{
				Type:      ActionUpdatePrimary,
				ClusterID: desiredCluster.Id,
				Spec:      &primaryUpdateSpec{Desired: desiredCluster, Actual: actualCluster},
			})
		} else if actualCluster.Status != "running" {
			actions = append(actions, Action{
				Type: ActionRecoverPrimary, ClusterID: desiredCluster.Id, Spec: actualCluster,
			})
		}

		actions = append(actions, diffPgHba(desiredCluster, actualCluster)...)
		actions = append(actions, diffReplicas(desiredCluster.Id, desiredCluster.Replicas, actualCluster.Replicas, primaryReplacement)...)
		pgBouncerActions := diffPgBouncer(desiredCluster, desiredPgBouncerConfig, pgBouncerConfigValid, actualCluster.PgBouncer)
		actions = append(actions, pgBouncerActions...)
		extensionActions := diffExtensions(desiredCluster, actualCluster)
		actions = append(actions, extensionActions...)
		actions = append(actions, diffPgBackRest(desiredCluster, actualCluster.Backup, primaryWillChange)...)
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
	if cluster.PgHba != nil {
		actions = append(actions, Action{Type: ActionUpdatePgHba, ClusterID: cluster.Id, Spec: &pgHbaUpdateSpec{Desired: cluster}})
	}

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
	if cluster.PgBackRest != nil {
		actions = append(actions, Action{Type: ActionCreatePgBackRest, ClusterID: cluster.Id, Spec: cluster})
	}

	return actions
}

func diffPgHba(desired *ClusterSpec, actual *ActualCluster) []Action {
	if desired.PgHba == nil || actual == nil || actual.ContainerId == "" {
		return nil
	}
	desiredRules := postgres.DesiredHBARules(desired)
	expectedReplicationCIDRs := []string(nil)
	if len(desired.Replicas) > 0 {
		expectedReplicationCIDRs = actual.NetworkCidrs
	}
	needsUpdate := !actual.PgHbaObserved || !postgres.RulesEqual(desiredRules, actual.PgHbaRules) ||
		!postgres.StringsEqual(expectedReplicationCIDRs, actual.PgHbaReplicationCidrs)
	for _, replica := range actual.Replicas {
		if replica != nil && replica.Status == "running" && (!replica.PgHbaObserved || !postgres.RulesEqual(desiredRules, replica.PgHbaRules)) {
			needsUpdate = true
		}
	}
	if !needsUpdate {
		return nil
	}
	return []Action{{Type: ActionUpdatePgHba, ClusterID: desired.Id, Spec: &pgHbaUpdateSpec{Desired: desired, Actual: actual}}}
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
	if cluster.Backup != nil {
		actions = append(actions, Action{Type: ActionDeletePgBackRest, ClusterID: cluster.Id, Spec: &pgBackRestDeleteSpec{
			Backup: cluster.Backup, DeleteCluster: true,
		}})
	}

	actions = append(actions, Action{
		Type:      ActionDeletePrimary,
		ClusterID: cluster.Id,
		Spec:      cluster,
	})

	return actions
}

func primaryNeedsUpdate(desired *ClusterSpec, actual *ActualCluster) bool {
	return primaryRequiresReplacement(desired, actual) || len(postgres.ChangedParameters(desired.Params, actual.AppliedParams)) > 0
}

func primaryRequiresReplacement(desired *ClusterSpec, actual *ActualCluster) bool {
	imageMissingPgBackRest := desired.PgBackRest != nil && !strings.HasPrefix(strings.TrimPrefix(actual.Image, "docker.io/library/"), "orca-postgres:")
	legacyConfig := actual.AppliedParams == nil && len(desired.Params) > 0
	return desired.Version != actual.Version || imageMissingPgBackRest || legacyConfig
}

func diffReplicas(clusterID string, desired []*ReplicaSpec, actual []*ActualReplica, primaryVersionChanged bool) []Action {
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
		if actualReplica.Status != "running" || primaryVersionChanged {
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

func diffPgBackRest(desired *ClusterSpec, actual *ActualBackup, forceUpdate ...bool) []Action {
	if desired.PgBackRest == nil && actual == nil {
		return nil
	}
	if desired.PgBackRest == nil {
		return []Action{{Type: ActionDeletePgBackRest, ClusterID: desired.Id, Spec: actual}}
	}
	state, err := pgbackrest.ReconciliationState(desired)
	if actual == nil {
		return []Action{{Type: ActionCreatePgBackRest, ClusterID: desired.Id, Spec: desired}}
	}
	if err != nil || actual.Config != state || len(forceUpdate) > 0 && forceUpdate[0] {
		return []Action{{Type: ActionUpdatePgBackRest, ClusterID: desired.Id, Spec: desired}}
	}
	return nil
}

func diffExtensions(desired *ClusterSpec, actual *ActualCluster) []Action {
	if actual == nil || actual.ContainerId == "" || actual.Status != "running" || actual.EnabledExtensions == nil {
		return nil
	}
	extensionActions, err := extensions.Diff(desired.EnabledExtensions, actual.EnabledExtensions)
	if err != nil {
		return []Action{{
			Type: ActionUpdateExtensions, ClusterID: desired.Id,
			Spec: &extensionUpdateSpec{
				Desired: append([]string(nil), desired.EnabledExtensions...), Actual: actual, DiffErr: err,
			},
		}}
	}
	if len(extensionActions) == 0 {
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
	config, err := pgbouncer.GeneratePgBouncerConfig(desired)
	return config, err == nil
}

func extensionUpdate(action Action) (*extensionUpdateSpec, error) {
	update, ok := action.Spec.(*extensionUpdateSpec)
	if !ok || update.Actual == nil || update.Actual.ContainerId == "" {
		return nil, fmt.Errorf("%s action requires extension update state", action.Type)
	}
	if update.DiffErr != nil {
		return nil, update.DiffErr
	}
	return update, nil
}
