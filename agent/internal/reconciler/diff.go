package reconciler

import (
	"fmt"
	"strings"

	orcadocker "github.com/swapnil404/orca/agent/internal/docker"
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
	// ActionObserveParameters reports a failure reading live PostgreSQL parameter state.
	ActionObserveParameters ActionType = "observe_parameters"
	// ActionCreatePgBackRest configures a new pgBackRest stanza and schedule.
	ActionCreatePgBackRest ActionType = "create_pgbackrest"
	// ActionUpdatePgBackRest updates an existing pgBackRest stanza and schedule.
	ActionUpdatePgBackRest ActionType = "update_pgbackrest"
	// ActionDeletePgBackRest disables pgBackRest for a retained primary.
	ActionDeletePgBackRest ActionType = "delete_pgbackrest"
	// ActionRunPgBackRestBackup reports an asynchronous scheduled backup attempt.
	ActionRunPgBackRestBackup ActionType = "run_pgbackrest_backup"
	// ActionRestartCluster restarts every managed container in a cluster.
	ActionRestartCluster ActionType = "restart_cluster"
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

type replicaDeleteSpec struct {
	Actual             *ActualReplica
	SkipPrimaryCleanup bool
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
		primaryRecreated := actualCluster.ContainerId == "" || primaryReplacement
		primaryWillChange := actualCluster.ContainerId == "" || primaryNeedsUpdate(desiredCluster, actualCluster)
		primaryActions := []Action{}
		if actualCluster.ContainerId == "" {
			primaryActions = append(primaryActions, Action{Type: ActionCreatePrimary, ClusterID: desiredCluster.Id, Spec: desiredCluster})
		} else if primaryNeedsUpdate(desiredCluster, actualCluster) {
			primaryActions = append(primaryActions, Action{
				Type:      ActionUpdatePrimary,
				ClusterID: desiredCluster.Id,
				Spec:      &primaryUpdateSpec{Desired: desiredCluster, Actual: actualCluster},
			})
		} else if actualCluster.Status != "running" {
			primaryActions = append(primaryActions, Action{
				Type: ActionRecoverPrimary, ClusterID: desiredCluster.Id, Spec: actualCluster,
			})
		}

		replicaActions := diffReplicas(desiredCluster, actualCluster.Replicas, primaryRecreated, actualCluster.ContainerId == "")
		pgBouncerActions := diffPgBouncer(desiredCluster, desiredPgBouncerConfig, pgBouncerConfigValid, actualCluster.PgBouncer, primaryRecreated)
		if primaryRecreated {
			actions = append(actions, actionsOfType(replicaActions, ActionDeleteReplica)...)
			actions = append(actions, actionsOfType(pgBouncerActions, ActionDeletePgBouncer)...)
			actions = append(actions, primaryActions...)
			actions = append(actions, diffPgHba(desiredCluster, actualCluster, true)...)
			actions = append(actions, actionsExceptType(replicaActions, ActionDeleteReplica)...)
			actions = append(actions, actionsExceptType(pgBouncerActions, ActionDeletePgBouncer)...)
		} else {
			actions = append(actions, primaryActions...)
			actions = append(actions, diffPgHba(desiredCluster, actualCluster, false)...)
			actions = append(actions, replicaActions...)
			actions = append(actions, pgBouncerActions...)
		}
		extensionActions := diffExtensions(desiredCluster, actualCluster, primaryRecreated)
		actions = append(actions, extensionActions...)
		actions = append(actions, diffPgBackRest(desiredCluster, actualCluster.Backup, primaryWillChange)...)
		if desiredCluster.RestartGeneration > actualCluster.AppliedRestartGeneration {
			actions = append(actions, Action{Type: ActionRestartCluster, ClusterID: desiredCluster.Id, Spec: desiredCluster})
		}
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

func diffPgHba(desired *ClusterSpec, actual *ActualCluster, primaryRecreated bool) []Action {
	if desired.PgHba == nil || actual == nil {
		return nil
	}
	if primaryRecreated {
		return []Action{{Type: ActionUpdatePgHba, ClusterID: desired.Id, Spec: &pgHbaUpdateSpec{Desired: desired}}}
	}
	if actual.ContainerId == "" {
		return nil
	}
	desiredRules := postgres.DesiredHBARules(desired)
	expectedReplicationCIDRs := []string(nil)
	expectedPoolCIDRs := []string(nil)
	if len(desired.Replicas) > 0 {
		expectedReplicationCIDRs = actual.NetworkCidrs
	}
	if desired.PgBouncer != nil {
		expectedPoolCIDRs = actual.NetworkCidrs
	}
	needsUpdate := !actual.PgHbaObserved || !postgres.RulesEqual(desiredRules, actual.PgHbaRules) ||
		!postgres.StringsEqual(expectedReplicationCIDRs, actual.PgHbaReplicationCidrs) || !postgres.StringsEqual(expectedPoolCIDRs, actual.PgHbaPoolCidrs)
	for _, replica := range actual.Replicas {
		if replica != nil && replica.Status == "running" && (!replica.PgHbaObserved || !postgres.RulesEqual(desiredRules, replica.PgHbaRules) || !postgres.StringsEqual(expectedPoolCIDRs, replica.PgHbaPoolCidrs)) {
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
	if primaryRequiresReplacement(desired, actual) || len(postgres.ChangedParameters(desired.Params, actual.AppliedParams)) > 0 {
		return true
	}
	for _, replica := range actual.Replicas {
		if replica != nil && len(postgres.ChangedParameters(desired.Params, replica.AppliedParams)) > 0 {
			return true
		}
	}
	return false
}

func primaryRequiresReplacement(desired *ClusterSpec, actual *ActualCluster) bool {
	imageMissingPgBackRest := desired.PgBackRest != nil && !strings.HasPrefix(strings.TrimPrefix(actual.Image, "docker.io/library/"), "orca-postgres:")
	repositoryMountMissing := desired.PgBackRest != nil && (actual.Backup == nil || !strings.Contains(actual.Backup.Config, "\n[orca-storage]\nrepo-bind="+desired.PgBackRest.RepoPath+"\n"))
	repositoryMountStale := desired.PgBackRest == nil && actual.Backup != nil && strings.Contains(actual.Backup.Config, "\n[orca-storage]\nrepo-bind=")
	return desired.Version != actual.Version || imageMissingPgBackRest || repositoryMountMissing || repositoryMountStale || actual.NetworkName != "orca-"+desired.Id+"-network"
}

func diffReplicas(cluster *ClusterSpec, actual []*ActualReplica, primaryRecreated, primaryMissing bool) []Action {
	actions := []Action{}
	desired := cluster.Replicas
	actualReplicas := make(map[string]*ActualReplica, len(actual))
	for _, replica := range actual {
		actualReplicas[replica.Id] = replica
	}

	for _, desiredReplica := range desired {
		actualReplica, exists := actualReplicas[desiredReplica.Id]
		if !exists {
			actions = append(actions, Action{
				Type:      ActionCreateReplica,
				ClusterID: cluster.Id,
				ReplicaID: desiredReplica.Id,
				Spec:      desiredReplica,
			})
			continue
		}
		legacyParameterConfig := actualReplica.AppliedParams == nil && len(cluster.Params) > 0
		if actualReplica.Status != "running" || primaryRecreated || legacyParameterConfig || actualReplica.NetworkName != "orca-"+cluster.Id+"-network" {
			actions = append(actions,
				deleteReplicaAction(cluster.Id, actualReplica, primaryMissing),
				Action{Type: ActionCreateReplica, ClusterID: cluster.Id, ReplicaID: desiredReplica.Id, Spec: desiredReplica},
			)
		}

		delete(actualReplicas, desiredReplica.Id)
	}

	for _, actualReplica := range actual {
		if _, exists := actualReplicas[actualReplica.Id]; exists {
			actions = append(actions, deleteReplicaAction(cluster.Id, actualReplica, primaryMissing))
		}
	}

	return actions
}

func diffPgBouncer(desired *ClusterSpec, desiredConfig string, configValid bool, actual *ActualPgBouncer, primaryRecreated bool) []Action {
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
	if primaryRecreated {
		return []Action{
			{Type: ActionDeletePgBouncer, ClusterID: desired.Id, Spec: actual},
			{Type: ActionCreatePgBouncer, ClusterID: desired.Id, Spec: desired},
		}
	}
	if !configValid || desiredConfig != actual.Config || actual.Status != "running" ||
		actual.NetworkName != "orca-"+desired.Id+"-network" || actual.PublishedAddress != desired.PgBouncer.PublishAddress || actual.PublishedPort != desired.PgBouncer.PublishPort {
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

func diffExtensions(desired *ClusterSpec, actual *ActualCluster, primaryRecreated bool) []Action {
	if primaryRecreated {
		primary, err := postgresPrimaryName(desired.Id)
		if err != nil {
			return []Action{{Type: ActionUpdateExtensions, ClusterID: desired.Id, Spec: &extensionUpdateSpec{Desired: desired.EnabledExtensions, DiffErr: err}}}
		}
		return diffExtensions(desired, &ActualCluster{
			ContainerId: primary, Status: "running", EnabledExtensions: actual.EnabledExtensions,
		}, false)
	}
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

func deleteReplicaAction(clusterID string, replica *ActualReplica, skipPrimaryCleanup bool) Action {
	return Action{Type: ActionDeleteReplica, ClusterID: clusterID, ReplicaID: replica.Id, Spec: &replicaDeleteSpec{
		Actual: replica, SkipPrimaryCleanup: skipPrimaryCleanup,
	}}
}

func actionsOfType(actions []Action, actionType ActionType) []Action {
	filtered := make([]Action, 0, len(actions))
	for _, action := range actions {
		if action.Type == actionType {
			filtered = append(filtered, action)
		}
	}
	return filtered
}

func actionsExceptType(actions []Action, actionType ActionType) []Action {
	filtered := make([]Action, 0, len(actions))
	for _, action := range actions {
		if action.Type != actionType {
			filtered = append(filtered, action)
		}
	}
	return filtered
}

func postgresPrimaryName(clusterID string) (string, error) {
	return orcadocker.ContainerName(orcadocker.ContainerSpec{ClusterID: clusterID, Kind: orcadocker.ContainerKindPrimary})
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
