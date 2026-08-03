package reconciler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	orcadocker "github.com/swapnil404/orca/agent/internal/docker"
	"github.com/swapnil404/orca/agent/internal/extensions"
	"github.com/swapnil404/orca/agent/internal/pgbackrest"
	"github.com/swapnil404/orca/agent/internal/pgbouncer"
	"github.com/swapnil404/orca/agent/internal/postgres"
)

// DockerClient is the Docker wrapper interface used by Apply.
type DockerClient = orcadocker.DockerClient

// ApplyStatus identifies the outcome of an apply action.
type ApplyStatus string

const (
	// ApplyStatusSuccess means the action completed successfully.
	ApplyStatusSuccess ApplyStatus = "success"
	// ApplyStatusFailed means the action was attempted and failed.
	ApplyStatusFailed ApplyStatus = "failed"
	// ApplyStatusSkippedDependency means the action was not attempted because a prerequisite failed.
	ApplyStatusSkippedDependency ApplyStatus = "skipped_due_to_dependency"
)

// ApplyResult reports the outcome of executing one action.
type ApplyResult struct {
	Action Action
	Status ApplyStatus
	Err    error
}

// MarshalJSON encodes an apply error as a readable string or null.
func (r ApplyResult) MarshalJSON() ([]byte, error) {
	var applyError *string
	if r.Err != nil {
		message := r.Err.Error()
		applyError = &message
	}

	return json.Marshal(struct {
		Action Action
		Status ApplyStatus
		Err    *string
	}{
		Action: r.Action,
		Status: r.Status,
		Err:    applyError,
	})
}

// Apply executes every action against Docker and reports each result.
func Apply(ctx context.Context, docker DockerClient, actions []Action, desiredStates ...*DesiredState) []ApplyResult {
	desired := &DesiredState{}
	if len(desiredStates) > 0 && desiredStates[0] != nil {
		desired = desiredStates[0]
	}
	return apply(ctx, docker, nil, actions, desired)
}

func apply(ctx context.Context, docker DockerClient, backups *pgbackrest.Scheduler, actions []Action, desired *DesiredState) []ApplyResult {
	results := make([]ApplyResult, 0, len(actions))
	failedReplicaDeletes := make(map[string]error)
	failedReplicaCreates := make(map[string]struct{})
	failedPrimaryActions := make(map[string]struct{})
	failedDependentDeletes := make(map[string]struct{})
	failedBackupDeletes := make(map[string]struct{})
	for _, action := range actions {
		key := action.ClusterID + "\x00" + action.ReplicaID
		if isPrimaryMutation(action.Type) {
			if _, blocked := failedDependentDeletes[action.ClusterID]; blocked {
				results = append(results, ApplyResult{Action: action, Status: ApplyStatusSkippedDependency})
				failedPrimaryActions[action.ClusterID] = struct{}{}
				continue
			}
		}
		if isPrimaryDependentAction(action.Type) {
			if _, blocked := failedPrimaryActions[action.ClusterID]; blocked {
				results = append(results, ApplyResult{Action: action, Status: ApplyStatusSkippedDependency})
				continue
			}
		}
		if action.Type == ActionCreateReplica {
			if _, blocked := failedReplicaDeletes[key]; blocked {
				results = append(results, ApplyResult{Action: action, Status: ApplyStatusSkippedDependency})
				continue
			}
		}
		if action.Type == ActionCreatePgBouncer || action.Type == ActionUpdatePgBouncer {
			if _, blocked := failedReplicaCreates[action.ClusterID]; blocked {
				results = append(results, ApplyResult{Action: action, Status: ApplyStatusSkippedDependency})
				continue
			}
		}
		if action.Type == ActionDeletePrimary {
			if _, blocked := failedDependentDeletes[action.ClusterID]; blocked {
				results = append(results, ApplyResult{Action: action, Status: ApplyStatusSkippedDependency})
				continue
			}
			if _, blocked := failedBackupDeletes[action.ClusterID]; blocked {
				results = append(results, ApplyResult{Action: action, Status: ApplyStatusSkippedDependency})
				continue
			}
		}
		err := applyAction(ctx, docker, backups, action, desired)
		results = append(results, ApplyResult{Action: action, Status: applyStatus(err), Err: err})
		if action.Type == ActionDeleteReplica && err != nil {
			failedReplicaDeletes[key] = err
		}
		if action.Type == ActionCreateReplica && err != nil {
			failedReplicaCreates[action.ClusterID] = struct{}{}
		}
		if isPrimaryDependentDelete(action.Type) && err != nil {
			failedDependentDeletes[action.ClusterID] = struct{}{}
		}
		if isPrimaryMutation(action.Type) && err != nil {
			failedPrimaryActions[action.ClusterID] = struct{}{}
		}
		if action.Type == ActionDeletePgBackRest && err != nil {
			failedBackupDeletes[action.ClusterID] = struct{}{}
		}
	}
	return results
}

func applyStatus(err error) ApplyStatus {
	if err != nil {
		return ApplyStatusFailed
	}
	return ApplyStatusSuccess
}

func isPrimaryMutation(actionType ActionType) bool {
	return actionType == ActionCreatePrimary || actionType == ActionUpdatePrimary || actionType == ActionRecoverPrimary
}

func isPrimaryDependentAction(actionType ActionType) bool {
	return actionType == ActionCreateReplica || actionType == ActionDeleteReplica || actionType == ActionCreatePgBouncer || actionType == ActionUpdatePgBouncer ||
		actionType == ActionUpdateExtensions || actionType == ActionUpdatePgHba || actionType == ActionCreatePgBackRest || actionType == ActionUpdatePgBackRest ||
		actionType == ActionRestartCluster
}

func isPrimaryDependentDelete(actionType ActionType) bool {
	return actionType == ActionDeleteReplica || actionType == ActionDeletePgBouncer
}

func applyAction(ctx context.Context, docker DockerClient, backups *pgbackrest.Scheduler, action Action, desired *DesiredState) error {
	if docker == nil {
		return errors.New("docker client is nil")
	}

	switch action.Type {
	case ActionCreatePrimary:
		spec, err := primaryContainerSpec(action)
		if err != nil {
			return err
		}
		cluster, _ := action.Spec.(*ClusterSpec)
		var params map[string]string
		if cluster != nil {
			params = cluster.Params
		}
		return createPrimary(ctx, docker, spec, params)
	case ActionUpdatePrimary:
		return updatePrimary(ctx, docker, action)
	case ActionRecoverPrimary:
		containerID, err := primaryContainerID(action)
		if err != nil {
			return err
		}
		return recoverPrimary(ctx, docker, containerID)
	case ActionCreateReplica:
		return createReplica(ctx, docker, action, desired)
	case ActionCreatePgBouncer:
		return createPgBouncer(ctx, docker, action)
	case ActionUpdatePgBouncer:
		return updatePgBouncer(ctx, docker, action)
	case ActionUpdateExtensions:
		return updateExtensions(ctx, docker, action)
	case ActionUpdatePgHba:
		return updatePgHba(ctx, docker, action)
	case ActionCreatePgBackRest, ActionUpdatePgBackRest:
		return configurePgBackRest(ctx, docker, backups, action)
	case ActionDeletePgBackRest:
		return deletePgBackRest(ctx, docker, backups, action)
	case ActionRestartCluster:
		return restartCluster(ctx, docker, action)
	case ActionDeletePrimary:
		cluster, ok := action.Spec.(*ActualCluster)
		if !ok {
			return errors.New("delete_primary action requires ActualCluster")
		}
		if cluster.ContainerId != "" {
			if err := stopAndRemove(ctx, docker, cluster.ContainerId); err != nil {
				return err
			}
		}
		if err := docker.RemoveNetwork(ctx, orcadocker.NetworkName(cluster.Id)); err != nil {
			return err
		}
		if err := docker.RemoveClusterData(ctx, cluster.Id); err != nil {
			return err
		}
		return docker.RemoveVolume(ctx, orcadocker.VolumeName(cluster.Id))
	case ActionDeleteReplica:
		containerID, err := replicaContainerID(action)
		if err != nil {
			return err
		}
		deleteSpec, _ := action.Spec.(*replicaDeleteSpec)
		if !desiredContainsCluster(desired, action.ClusterID) || deleteSpec != nil && deleteSpec.SkipPrimaryCleanup {
			return stopAndRemove(ctx, docker, containerID)
		}
		replicaDocker, ok := docker.(postgres.ReplicaDockerClient)
		if !ok {
			return errors.New("docker client does not support replica cleanup")
		}
		return postgres.DeleteReplica(ctx, replicaDocker, action.ClusterID, action.ReplicaID, containerID)
	case ActionDeletePgBouncer:
		containerID, err := pgBouncerContainerID(action)
		if err != nil {
			return err
		}
		return stopAndRemove(ctx, docker, containerID)
	default:
		return fmt.Errorf("unknown action type %q", action.Type)
	}
}

func restartCluster(ctx context.Context, docker DockerClient, action Action) error {
	cluster, ok := action.Spec.(*ClusterSpec)
	if !ok || cluster == nil {
		return errors.New("restart_cluster action requires desired cluster state")
	}
	containers, err := docker.ListOrcaContainers(ctx)
	if err != nil {
		return err
	}
	targets := make([]orcadocker.ContainerInfo, 0)
	for _, container := range containers {
		if container.ClusterID == action.ClusterID && container.Kind != orcadocker.ContainerKindPgBackRest {
			targets = append(targets, container)
		}
	}
	if len(targets) == 0 {
		return fmt.Errorf("cluster %q has no managed containers to restart", action.ClusterID)
	}
	sort.Slice(targets, func(i, j int) bool {
		return restartOrder(targets[i].Kind) < restartOrder(targets[j].Kind)
	})
	stopped := make([]orcadocker.ContainerInfo, 0, len(targets))
	for _, container := range targets {
		if container.Status == "running" {
			if err := docker.StopContainer(ctx, container.ID); err != nil {
				return errors.Join(err, startContainers(ctx, docker, stopped))
			}
		}
		stopped = append(stopped, container)
	}
	primaryIndex := -1
	for index, container := range stopped {
		if container.Kind == orcadocker.ContainerKindPrimary {
			primaryIndex = index
			break
		}
	}
	if primaryIndex < 0 {
		return errors.Join(errors.New("managed primary container is missing"), startContainers(ctx, docker, stopped))
	}
	configDocker, ok := docker.(primaryConfigDockerClient)
	if !ok {
		return errors.Join(errors.New("docker client does not support PostgreSQL readiness checks"), startContainers(ctx, docker, stopped))
	}
	if err := docker.StartContainer(ctx, stopped[primaryIndex].ID); err != nil {
		return fmt.Errorf("start primary after project restart: %w", err)
	}
	if err := postgres.WaitForPrimaryReady(ctx, configDocker, stopped[primaryIndex].ID); err != nil {
		return fmt.Errorf("wait for primary after project restart: %w", err)
	}
	dependents := append([]orcadocker.ContainerInfo(nil), stopped[:primaryIndex]...)
	dependents = append(dependents, stopped[primaryIndex+1:]...)
	if err := startContainers(ctx, docker, dependents); err != nil {
		return err
	}
	stateDocker, ok := docker.(interface {
		WriteConfig(context.Context, string, *orcadocker.ConfigMount) error
	})
	if !ok {
		return errors.New("docker client does not support restart state persistence")
	}
	return stateDocker.WriteConfig(ctx, action.ClusterID, &orcadocker.ConfigMount{
		RelativePath: orcadocker.RestartAppliedRelativePath,
		Content:      strconv.FormatUint(cluster.RestartGeneration, 10),
	})
}

func startContainers(ctx context.Context, docker DockerClient, containers []orcadocker.ContainerInfo) error {
	var startErr error
	for index := len(containers) - 1; index >= 0; index-- {
		startErr = errors.Join(startErr, docker.StartContainer(ctx, containers[index].ID))
	}
	return startErr
}

func restartOrder(kind orcadocker.ContainerKind) int {
	switch kind {
	case orcadocker.ContainerKindPgBouncer:
		return 0
	case orcadocker.ContainerKindReplica:
		return 1
	case orcadocker.ContainerKindPrimary:
		return 2
	default:
		return 3
	}
}

func desiredContainsCluster(desired *DesiredState, clusterID string) bool {
	if desired == nil {
		return false
	}
	for _, cluster := range desired.Clusters {
		if cluster != nil && cluster.Id == clusterID {
			return true
		}
	}
	return false
}

type replicaDockerClient interface {
	postgres.DockerClient
	postgres.ReplicaDockerClient
}

type pgBouncerDockerClient interface {
	DockerClient
	pgbouncer.ConsoleExecutor
	WriteConfig(ctx context.Context, clusterID string, config *orcadocker.ConfigMount) error
}

type extensionDockerClient interface {
	DockerClient
	extensions.PrimaryExecutor
}

type pgHbaDockerClient interface {
	DockerClient
	postgres.HBAExecutor
}

func updatePgHba(ctx context.Context, docker DockerClient, action Action) error {
	update, ok := action.Spec.(*pgHbaUpdateSpec)
	if !ok || update.Desired == nil {
		return errors.New("update_pg_hba action requires desired cluster state")
	}
	client, ok := docker.(pgHbaDockerClient)
	if !ok {
		return errors.New("docker client does not support pg_hba reconciliation")
	}
	slog.Info("reloading PostgreSQL authentication configuration", "cluster_id", action.ClusterID)
	return postgres.ApplyHBA(ctx, client, update.Desired, update.Actual)
}

type pgBackRestDockerClient interface {
	DockerClient
	pgbackrest.PrimaryExecutor
	WriteConfig(context.Context, string, *orcadocker.ConfigMount) error
}

func configurePgBackRest(ctx context.Context, docker DockerClient, backups *pgbackrest.Scheduler, action Action) error {
	cluster, ok := action.Spec.(*ClusterSpec)
	if !ok || cluster.PgBackRest == nil {
		return fmt.Errorf("%s action requires pgBackRest cluster state", action.Type)
	}
	client, ok := docker.(pgBackRestDockerClient)
	if !ok {
		return errors.New("docker client does not support pgBackRest reconciliation")
	}
	state, err := pgbackrest.ReconciliationState(cluster)
	if err != nil {
		return err
	}
	if err := pgbackrest.InstallConfig(ctx, client, cluster); err != nil {
		return fmt.Errorf("install config: %w", err)
	}
	if err := pgbackrest.ConfigureWALArchiving(ctx, client, cluster); err != nil {
		return fmt.Errorf("configure WAL archiving: %w", err)
	}
	if err := pgbackrest.InitializeStanza(ctx, client, cluster); err != nil {
		return fmt.Errorf("initialize stanza: %w", err)
	}
	if err := client.WriteConfig(ctx, cluster.Id, &orcadocker.ConfigMount{
		RelativePath: orcadocker.PgBackRestAppliedConfigRelativePath,
		Content:      state,
	}); err != nil {
		return err
	}
	if backups != nil {
		backups.SetSchedule(cluster)
	}
	return nil
}

func deletePgBackRest(ctx context.Context, docker DockerClient, backups *pgbackrest.Scheduler, action Action) error {
	client, ok := docker.(pgBackRestDockerClient)
	if !ok {
		return errors.New("docker client does not support pgBackRest reconciliation")
	}
	clusterID := action.ClusterID
	if backups != nil {
		backups.RemoveSchedule(clusterID)
	}
	if spec, ok := action.Spec.(*pgBackRestDeleteSpec); ok && spec.DeleteCluster {
		return nil
	}
	if err := pgbackrest.DisableWALArchiving(ctx, client, clusterID); err != nil {
		return err
	}
	if err := pgbackrest.RemoveConfig(ctx, client, clusterID); err != nil {
		return err
	}
	return client.WriteConfig(ctx, clusterID, &orcadocker.ConfigMount{
		RelativePath: orcadocker.PgBackRestAppliedConfigRelativePath,
	})
}

type primaryConfigDockerClient interface {
	DockerClient
	WriteConfig(context.Context, string, *orcadocker.ConfigMount) error
	ExecContainer(context.Context, string, []string) (string, error)
}

type postgresNodeUpdate struct {
	containerID string
	dataPath    string
	configPath  string
	appliedPath string
	applied     map[string]string
}

func createReplica(ctx context.Context, docker DockerClient, action Action, desired *DesiredState) error {
	replicaDocker, ok := docker.(replicaDockerClient)
	if !ok {
		return errors.New("docker client does not support replica provisioning")
	}
	cluster, err := desiredReplica(desired, action.ClusterID, action.ReplicaID)
	if err != nil {
		return err
	}
	if err := postgres.ConfigurePrimaryReplication(ctx, replicaDocker, cluster); err != nil {
		return fmt.Errorf("configure primary replication: %w", err)
	}
	primary, err := orcadocker.ContainerName(orcadocker.ContainerSpec{
		ClusterID: action.ClusterID,
		Kind:      orcadocker.ContainerKindPrimary,
	})
	if err != nil {
		return err
	}
	replicaID, err := postgres.CreateReplica(ctx, replicaDocker, postgres.ReplicaSpec{
		ClusterID:       cluster.Id,
		ReplicaID:       action.ReplicaID,
		PostgresVersion: cluster.Version,
		Params:          cluster.Params,
		Primary: postgres.PrimaryConnectionInfo{
			Host: primary,
		},
	})
	if err != nil {
		return err
	}
	if cluster.PgHba != nil {
		if err := postgres.ApplyReplicaHBA(ctx, replicaDocker, cluster, replicaID); err != nil {
			return fmt.Errorf("apply replica pg_hba.conf: %w", err)
		}
	}
	return markReplicaParamsApplied(ctx, replicaDocker, replicaID, cluster, action.ReplicaID)
}

func desiredReplica(desired *DesiredState, clusterID, replicaID string) (*ClusterSpec, error) {
	for _, cluster := range desired.Clusters {
		if cluster == nil || cluster.Id != clusterID {
			continue
		}
		for _, replica := range cluster.Replicas {
			if replica != nil && replica.Id == replicaID {
				return cluster, nil
			}
		}
		return nil, fmt.Errorf("replica %q is not desired for cluster %q", replicaID, clusterID)
	}
	return nil, fmt.Errorf("cluster %q is not desired", clusterID)
}

func createAndStart(ctx context.Context, docker DockerClient, spec orcadocker.ContainerSpec, specErr error) error {
	if specErr != nil {
		return specErr
	}

	containerID, err := docker.CreateContainer(ctx, spec)
	if err != nil {
		return err
	}

	return docker.StartContainer(ctx, containerID)
}

func createPrimary(ctx context.Context, docker DockerClient, spec orcadocker.ContainerSpec, params map[string]string) error {
	credentials, ok := docker.(clusterCredentialDockerClient)
	if !ok {
		return errors.New("docker client does not support cluster credentials")
	}
	password, err := credentials.EnsureClusterPassword(ctx, spec.ClusterID)
	if err != nil {
		return err
	}
	if err := credentials.WriteConfig(ctx, spec.ClusterID, &orcadocker.ConfigMount{
		RelativePath: "postgres/password", Content: password + "\n", Mode: 0o600,
	}); err != nil {
		return fmt.Errorf("write PostgreSQL password file: %w", err)
	}
	spec.Env = append(spec.Env, "POSTGRES_PASSWORD_FILE=/etc/orca/password")
	baseline, err := postgres.RenderConfig(spec.ClusterID, nil)
	if err != nil {
		return err
	}
	spec.Config.Content = baseline
	containerID, err := docker.CreateContainer(ctx, spec)
	if err != nil {
		return err
	}
	if err := docker.StartContainer(ctx, containerID); err != nil {
		return err
	}
	configDocker, ok := docker.(primaryConfigDockerClient)
	if !ok {
		return errors.New("docker client does not support PostgreSQL configuration updates")
	}
	if err := postgres.WaitForPrimaryReady(ctx, configDocker, containerID); err != nil {
		return err
	}
	if err := applyPostgresConfig(ctx, configDocker, docker, containerID, spec.ClusterID, orcadocker.VolumeMountPath(spec.ClusterID)+"/primary", orcadocker.PostgresConfigRelativePath, nil, params); err != nil {
		return err
	}
	if err := writeAppliedParams(ctx, configDocker, spec.ClusterID, orcadocker.PostgresAppliedConfigRelativePath, orcadocker.VolumeMountPath(spec.ClusterID)+"/primary", params); err != nil {
		return err
	}
	return synchronizePostgresPassword(ctx, credentials, containerID, password)
}

func recoverPrimary(ctx context.Context, docker DockerClient, containerID string) error {
	configDocker, ok := docker.(primaryConfigDockerClient)
	if !ok {
		return errors.New("docker client does not support PostgreSQL readiness checks")
	}
	if err := docker.StartContainer(ctx, containerID); err != nil {
		return err
	}
	return postgres.WaitForPrimaryReady(ctx, configDocker, containerID)
}

func stopAndRemove(ctx context.Context, docker DockerClient, containerID string) error {
	if containerID == "" {
		return errors.New("container ID is required")
	}
	if err := docker.StopContainer(ctx, containerID); err != nil {
		return err
	}

	return docker.RemoveContainer(ctx, containerID)
}

func updatePrimary(ctx context.Context, docker DockerClient, action Action) error {
	update, ok := action.Spec.(*primaryUpdateSpec)
	if !ok || update.Desired == nil || update.Actual == nil || update.Actual.ContainerId == "" {
		return errors.New("update_primary action requires desired and actual primary state")
	}
	spec, err := primaryContainerSpec(action)
	if err != nil {
		return err
	}

	if primaryRequiresReplacement(update.Desired, update.Actual) {
		if err := stopAndRemove(ctx, docker, update.Actual.ContainerId); err != nil {
			return fmt.Errorf("remove existing primary: %w", err)
		}
		return createPrimary(ctx, docker, spec, update.Desired.Params)
	}

	configDocker, ok := docker.(primaryConfigDockerClient)
	if !ok {
		return errors.New("docker client does not support PostgreSQL configuration updates")
	}
	if update.Actual.Status != "running" {
		if err := docker.StartContainer(ctx, update.Actual.ContainerId); err != nil {
			return err
		}
		if err := postgres.WaitForPrimaryReady(ctx, configDocker, update.Actual.ContainerId); err != nil {
			return err
		}
	}
	nodes := []postgresNodeUpdate{{
		containerID: update.Actual.ContainerId,
		dataPath:    orcadocker.VolumeMountPath(action.ClusterID) + "/primary",
		configPath:  orcadocker.PostgresConfigRelativePath,
		appliedPath: orcadocker.PostgresAppliedConfigRelativePath,
		applied:     update.Actual.AppliedParams,
	}}
	for _, replica := range update.Actual.Replicas {
		if replica == nil || replica.ContainerId == "" {
			continue
		}
		if replica.AppliedParams == nil && len(update.Desired.Params) > 0 {
			continue
		}
		identity, err := postgres.DeriveReplicaIdentity(action.ClusterID, replica.Id)
		if err != nil {
			return err
		}
		nodes = append(nodes, postgresNodeUpdate{
			containerID: replica.ContainerId, dataPath: identity.DataPath,
			configPath:  orcadocker.PostgresReplicaConfigRelativePath(replica.Id),
			appliedPath: orcadocker.PostgresReplicaAppliedConfigRelativePath(replica.Id),
			applied:     replica.AppliedParams,
		})
	}
	for _, node := range nodes {
		if err := preflightPostgresConfig(ctx, configDocker, node.containerID, node.dataPath, node.applied, update.Desired.Params); err != nil {
			return err
		}
	}
	for index, node := range nodes {
		if err := applyPostgresConfig(ctx, configDocker, docker, node.containerID, action.ClusterID, node.dataPath, node.configPath, node.applied, update.Desired.Params); err != nil {
			rollbackErr := rollbackPostgresNodes(context.WithoutCancel(ctx), configDocker, docker, action.ClusterID, nodes[:index+1])
			return errors.Join(err, rollbackErr)
		}
	}
	for _, node := range nodes {
		if err := writeAppliedParams(ctx, configDocker, action.ClusterID, node.appliedPath, node.dataPath, update.Desired.Params); err != nil {
			rollbackErr := rollbackPostgresNodes(context.WithoutCancel(ctx), configDocker, docker, action.ClusterID, nodes)
			return errors.Join(err, rollbackErr)
		}
	}
	return nil
}

func preflightPostgresConfig(ctx context.Context, configDocker primaryConfigDockerClient, containerID, dataPath string, applied, desired map[string]string) error {
	changed := postgres.ChangedParameters(desired, applied)
	metadata, err := postgres.InspectParameters(ctx, configDocker, containerID, changed)
	if err != nil {
		return err
	}
	if _, err := postgres.ClassifyConfigUpdate(changed, metadata); err != nil {
		return err
	}
	return postgres.ValidateParameterValues(ctx, configDocker, containerID, dataPath, desired)
}

func rollbackPostgresNodes(ctx context.Context, configDocker primaryConfigDockerClient, docker DockerClient, clusterID string, nodes []postgresNodeUpdate) error {
	var rollbackErr error
	for index := len(nodes) - 1; index >= 0; index-- {
		node := nodes[index]
		rollbackErr = errors.Join(rollbackErr, rollbackPostgresNode(ctx, configDocker, docker, node.containerID, clusterID, node.dataPath, node.configPath, node.applied))
		rollbackErr = errors.Join(rollbackErr, writeAppliedParams(ctx, configDocker, clusterID, node.appliedPath, node.dataPath, node.applied))
	}
	return rollbackErr
}

func rollbackPostgresNode(ctx context.Context, configDocker primaryConfigDockerClient, docker DockerClient, containerID, clusterID, dataPath, relativePath string, applied map[string]string) error {
	config, err := postgres.RenderNodeConfig(clusterID, dataPath, applied)
	if err != nil {
		return err
	}
	writeErr := configDocker.WriteConfig(ctx, clusterID, &orcadocker.ConfigMount{RelativePath: relativePath, ContainerPath: orcadocker.PostgresConfigContainerPath, Content: config})
	if writeErr != nil {
		return writeErr
	}
	stopErr := docker.StopContainer(ctx, containerID)
	startErr := docker.StartContainer(ctx, containerID)
	if stopErr != nil || startErr != nil {
		return errors.Join(stopErr, startErr)
	}
	return postgres.WaitForConfigApplied(ctx, configDocker, containerID, len(applied))
}

func applyPostgresConfig(ctx context.Context, configDocker primaryConfigDockerClient, docker DockerClient, containerID, clusterID, dataPath, relativePath string, applied, desired map[string]string) error {
	changed := postgres.ChangedParameters(desired, applied)
	metadata, err := postgres.InspectParameters(ctx, configDocker, containerID, changed)
	if err != nil {
		return err
	}
	method, err := postgres.ClassifyConfigUpdate(changed, metadata)
	if err != nil {
		return err
	}
	if err := postgres.ValidateParameterValues(ctx, configDocker, containerID, dataPath, desired); err != nil {
		return err
	}
	config, err := postgres.RenderNodeConfig(clusterID, dataPath, desired)
	if err != nil {
		return err
	}
	if err := configDocker.WriteConfig(ctx, clusterID, &orcadocker.ConfigMount{RelativePath: relativePath, ContainerPath: orcadocker.PostgresConfigContainerPath, Content: config}); err != nil {
		return fmt.Errorf("write PostgreSQL config: %w", err)
	}
	if len(changed) > 0 {
		switch method {
		case postgres.ConfigUpdateReload:
			slog.Info("reloading PostgreSQL configuration", "cluster_id", clusterID, "container_id", containerID)
			if _, err := configDocker.ExecContainer(ctx, containerID, []string{"psql", "--username", "postgres", "--dbname", "postgres", "--tuples-only", "--no-align", "--command", "SELECT pg_reload_conf();"}); err != nil {
				return fmt.Errorf("reload PostgreSQL config: %w", err)
			}
		case postgres.ConfigUpdateRestart:
			slog.Info("restarting PostgreSQL for configuration change", "cluster_id", clusterID, "container_id", containerID)
			if err := docker.StopContainer(ctx, containerID); err != nil {
				return err
			}
			if err := docker.StartContainer(ctx, containerID); err != nil {
				return err
			}
		default:
			return errors.New("unknown PostgreSQL config update method")
		}
	}
	return postgres.WaitForConfigApplied(ctx, configDocker, containerID, len(desired))
}

func markReplicaParamsApplied(ctx context.Context, docker replicaDockerClient, containerID string, cluster *ClusterSpec, replicaID string) error {
	if err := postgres.WaitForConfigApplied(ctx, docker, containerID, len(cluster.Params)); err != nil {
		return err
	}
	identity, err := postgres.DeriveReplicaIdentity(cluster.Id, replicaID)
	if err != nil {
		return err
	}
	configDocker, ok := any(docker).(interface {
		WriteConfig(context.Context, string, *orcadocker.ConfigMount) error
	})
	if !ok {
		return errors.New("docker client does not support replica parameter state")
	}
	return writeAppliedParams(ctx, configDocker, cluster.Id, orcadocker.PostgresReplicaAppliedConfigRelativePath(replicaID), identity.DataPath, cluster.Params)
}

func writeAppliedParams(ctx context.Context, configDocker interface {
	WriteConfig(context.Context, string, *orcadocker.ConfigMount) error
}, clusterID, relativePath, dataPath string, params map[string]string) error {
	config, err := postgres.RenderNodeConfig(clusterID, dataPath, params)
	if err != nil {
		return err
	}
	return configDocker.WriteConfig(ctx, clusterID, &orcadocker.ConfigMount{RelativePath: relativePath, Content: config})
}

func updatePgBouncer(ctx context.Context, docker DockerClient, action Action) error {
	update, ok := action.Spec.(*pgBouncerUpdateSpec)
	if !ok || update.Desired == nil || update.Actual == nil {
		return errors.New("update_pgbouncer action requires desired and actual PgBouncer state")
	}
	spec, err := pgBouncerContainerSpec(action)
	if err != nil {
		return err
	}
	pgBouncerDocker, ok := docker.(pgBouncerDockerClient)
	if !ok {
		return errors.New("docker client does not support PgBouncer updates")
	}
	if err := configurePgBouncerAuthentication(ctx, docker, action.ClusterID); err != nil {
		return err
	}
	if update.Actual.NetworkName != orcadocker.NetworkName(action.ClusterID) ||
		update.Actual.PublishedAddress != update.Desired.PgBouncer.PublishAddress ||
		update.Actual.PublishedPort != update.Desired.PgBouncer.PublishPort {
		if update.Actual.ContainerId != "" {
			if err := stopAndRemove(ctx, docker, update.Actual.ContainerId); err != nil {
				return err
			}
		}
		return createAndStart(ctx, docker, spec, nil)
	}

	changed, parseErr := pgbouncer.ChangedConfigKeys(update.Actual.Config, spec.Config.Content)
	method := pgbouncer.UpdateMethodRestart
	if parseErr == nil && update.Actual.Status == "running" {
		method = pgbouncer.ClassifyConfigUpdate(changed)
	}
	if err := pgBouncerDocker.WriteConfig(ctx, action.ClusterID, spec.Config); err != nil {
		return fmt.Errorf("write PgBouncer config: %w", err)
	}

	switch method {
	case pgbouncer.UpdateMethodReload:
		slog.Info("reloading PgBouncer configuration", "cluster_id", action.ClusterID)
		if err := pgbouncer.ReloadConfig(ctx, pgBouncerDocker, update.Actual.ContainerId); err != nil {
			rollbackErr := pgBouncerDocker.WriteConfig(ctx, action.ClusterID, &orcadocker.ConfigMount{
				RelativePath:  spec.Config.RelativePath,
				ContainerPath: spec.Config.ContainerPath,
				Content:       update.Actual.Config,
			})
			return errors.Join(err, rollbackErr)
		}
		return nil
	case pgbouncer.UpdateMethodRestart:
		slog.Info("restarting PgBouncer for configuration change", "cluster_id", action.ClusterID)
		if err := docker.StopContainer(ctx, update.Actual.ContainerId); err != nil {
			rollbackErr := pgBouncerDocker.WriteConfig(context.WithoutCancel(ctx), action.ClusterID, previousPgBouncerConfig(spec, update.Actual.Config))
			return errors.Join(err, rollbackErr)
		}
		if err := docker.StartContainer(ctx, update.Actual.ContainerId); err != nil {
			recoveryCtx := context.WithoutCancel(ctx)
			rollbackErr := pgBouncerDocker.WriteConfig(recoveryCtx, action.ClusterID, previousPgBouncerConfig(spec, update.Actual.Config))
			recoveryErr := docker.StartContainer(recoveryCtx, update.Actual.ContainerId)
			return errors.Join(err, rollbackErr, recoveryErr)
		}
		return nil
	default:
		return fmt.Errorf("unknown PgBouncer update method %q", method)
	}
}

func previousPgBouncerConfig(spec orcadocker.ContainerSpec, content string) *orcadocker.ConfigMount {
	return &orcadocker.ConfigMount{
		RelativePath:  spec.Config.RelativePath,
		ContainerPath: spec.Config.ContainerPath,
		Content:       content,
	}
}

func updateExtensions(ctx context.Context, docker DockerClient, action Action) error {
	update, err := extensionUpdate(action)
	if err != nil {
		return err
	}
	extensionDocker, ok := docker.(extensionDockerClient)
	if !ok {
		return errors.New("docker client does not support extension reconciliation")
	}

	results := extensions.Apply(ctx, extensionDocker, update.Actual.ContainerId, update.Desired, update.Actions)
	var applyErr error
	for _, result := range results {
		applyErr = errors.Join(applyErr, result.Err)
	}
	return applyErr
}

func primaryContainerSpec(action Action) (orcadocker.ContainerSpec, error) {
	if spec, ok := action.Spec.(orcadocker.ContainerSpec); ok {
		return spec, nil
	}

	cluster, ok := action.Spec.(*ClusterSpec)
	if update, updateOK := action.Spec.(*primaryUpdateSpec); updateOK {
		cluster, ok = update.Desired, update.Desired != nil
	}
	if !ok {
		return orcadocker.ContainerSpec{}, fmt.Errorf("%s action requires ClusterSpec", action.Type)
	}

	config, err := postgres.RenderConfig(cluster.Id, cluster.Params)
	if err != nil {
		return orcadocker.ContainerSpec{}, err
	}
	spec := orcadocker.ContainerSpec{
		ClusterID: cluster.Id,
		Kind:      orcadocker.ContainerKindPrimary,
		Image:     primaryImage(cluster),
		Env: []string{
			"POSTGRES_HOST_AUTH_METHOD=reject",
			"PGDATA=" + orcadocker.VolumeMountPath(cluster.Id) + "/primary",
		},
		Command:   []string{"postgres", "-c", "config_file=" + orcadocker.PostgresConfigContainerPath},
		UseVolume: true,
		Config: &orcadocker.ConfigMount{
			RelativePath: orcadocker.PostgresConfigRelativePath, ContainerPath: orcadocker.PostgresConfigContainerPath, Content: config,
		},
	}
	if cluster.PgBackRest != nil {
		spec.Binds = []orcadocker.BindMount{{Source: cluster.PgBackRest.RepoPath, Path: cluster.PgBackRest.RepoPath, Create: true}}
	}
	return spec, nil
}

func pgBouncerDesiredCluster(spec any) (*ClusterSpec, bool) {
	if cluster, ok := spec.(*ClusterSpec); ok {
		return cluster, true
	}
	if update, ok := spec.(*pgBouncerUpdateSpec); ok && update.Desired != nil {
		return update.Desired, true
	}
	return nil, false
}

func replicaContainerSpec(action Action) (orcadocker.ContainerSpec, error) {
	if spec, ok := action.Spec.(orcadocker.ContainerSpec); ok {
		return spec, nil
	}

	replica, ok := action.Spec.(*ReplicaSpec)
	if !ok {
		return orcadocker.ContainerSpec{}, errors.New("create_replica action requires ReplicaSpec")
	}

	identity, err := postgres.DeriveReplicaIdentity(action.ClusterID, replica.Id)
	if err != nil {
		return orcadocker.ContainerSpec{}, err
	}
	containerSpec := orcadocker.ContainerSpec{
		ClusterID: action.ClusterID,
		Kind:      orcadocker.ContainerKindReplica,
		ReplicaID: replica.Id,
		Image:     postgresImage(""),
		Env: []string{
			"POSTGRES_HOST_AUTH_METHOD=reject",
			"PGDATA=" + identity.DataPath,
		},
		UseVolume: true,
	}
	name, err := orcadocker.ContainerName(containerSpec)
	if err != nil {
		return orcadocker.ContainerSpec{}, err
	}
	if name != identity.ContainerName {
		return orcadocker.ContainerSpec{}, errors.New("replica container identity is inconsistent")
	}
	return containerSpec, nil
}

func pgBouncerContainerSpec(action Action) (orcadocker.ContainerSpec, error) {
	if spec, ok := action.Spec.(orcadocker.ContainerSpec); ok {
		return spec, nil
	}
	cluster, ok := pgBouncerDesiredCluster(action.Spec)
	if !ok {
		return orcadocker.ContainerSpec{}, fmt.Errorf("%s action requires ClusterSpec", action.Type)
	}
	config, err := pgbouncer.GeneratePgBouncerConfig(cluster)
	if err != nil {
		return orcadocker.ContainerSpec{}, err
	}

	return orcadocker.ContainerSpec{
		ClusterID: action.ClusterID,
		Kind:      orcadocker.ContainerKindPgBouncer,
		Image:     "edoburu/pgbouncer:v1.25.2-p0",
		Ports: []orcadocker.PublishedPort{{
			ContainerPort: 6432,
			HostAddress:   cluster.PgBouncer.PublishAddress,
			HostPort:      uint16(cluster.PgBouncer.PublishPort),
		}},
		Config: &orcadocker.ConfigMount{
			RelativePath:  orcadocker.PgBouncerConfigRelativePath,
			ContainerPath: orcadocker.PgBouncerConfigContainerPath,
			Content:       config,
		},
	}, nil
}

type clusterCredentialDockerClient interface {
	EnsureClusterPassword(context.Context, string) (string, error)
	ExecContainer(context.Context, string, []string) (string, error)
	WriteConfig(context.Context, string, *orcadocker.ConfigMount) error
}

func createPgBouncer(ctx context.Context, docker DockerClient, action Action) error {
	if err := configurePgBouncerAuthentication(ctx, docker, action.ClusterID); err != nil {
		return err
	}
	spec, err := pgBouncerContainerSpec(action)
	return createAndStart(ctx, docker, spec, err)
}

func configurePgBouncerAuthentication(ctx context.Context, docker DockerClient, clusterID string) error {
	credentials, ok := docker.(clusterCredentialDockerClient)
	if !ok {
		return errors.New("docker client does not support cluster credentials")
	}
	password, err := credentials.EnsureClusterPassword(ctx, clusterID)
	if err != nil {
		return err
	}
	primary, err := orcadocker.ContainerName(orcadocker.ContainerSpec{ClusterID: clusterID, Kind: orcadocker.ContainerKindPrimary})
	if err != nil {
		return err
	}
	if err := synchronizePostgresPassword(ctx, credentials, primary, password); err != nil {
		return err
	}
	verifier, err := credentials.ExecContainer(ctx, primary, []string{
		"psql", "--username", "postgres", "--dbname", "postgres", "--tuples-only", "--no-align",
		"--command", "SELECT rolpassword FROM pg_authid WHERE rolname = 'postgres';",
	})
	if err != nil {
		return fmt.Errorf("read PostgreSQL SCRAM verifier: %w", err)
	}
	verifier = strings.TrimSpace(verifier)
	if !strings.HasPrefix(verifier, "SCRAM-SHA-256$") {
		return errors.New("PostgreSQL did not produce a SCRAM verifier")
	}
	if err := credentials.WriteConfig(ctx, clusterID, &orcadocker.ConfigMount{
		RelativePath: orcadocker.PgBouncerAuthRelativePath,
		Content:      fmt.Sprintf("\"postgres\" \"%s\"\n\"pgbouncer\" \"\"\n", verifier),
	}); err != nil {
		return fmt.Errorf("write PgBouncer auth file: %w", err)
	}
	return credentials.WriteConfig(ctx, clusterID, &orcadocker.ConfigMount{
		RelativePath: orcadocker.PgBouncerHbaRelativePath,
		Content: "local pgbouncer pgbouncer trust\n" +
			"host all all 0.0.0.0/0 scram-sha-256\n" +
			"host all all ::/0 scram-sha-256\n",
	})
}

func synchronizePostgresPassword(ctx context.Context, docker clusterCredentialDockerClient, containerID, password string) error {
	if password == "" || strings.ContainsAny(password, "'\\") {
		return errors.New("invalid generated PostgreSQL password")
	}
	_, err := docker.ExecContainer(ctx, containerID, []string{
		"psql", "--username", "postgres", "--dbname", "postgres", "--set", "ON_ERROR_STOP=1",
		"--command", "SET password_encryption = 'scram-sha-256'; ALTER ROLE postgres PASSWORD '" + password + "';",
	})
	if err != nil {
		return fmt.Errorf("synchronize PostgreSQL password: %w", err)
	}
	return nil
}

func primaryContainerID(action Action) (string, error) {
	cluster, ok := action.Spec.(*ActualCluster)
	if !ok {
		return "", fmt.Errorf("%s action requires ActualCluster", action.Type)
	}

	return cluster.ContainerId, nil
}

func replicaContainerID(action Action) (string, error) {
	replica, ok := action.Spec.(*ActualReplica)
	if deleteSpec, deleteOK := action.Spec.(*replicaDeleteSpec); deleteOK {
		replica, ok = deleteSpec.Actual, deleteSpec.Actual != nil
	}
	if !ok || replica == nil {
		return "", errors.New("delete_replica action requires ActualReplica")
	}

	return replica.ContainerId, nil
}

func pgBouncerContainerID(action Action) (string, error) {
	pgBouncer, ok := action.Spec.(*ActualPgBouncer)
	if !ok {
		return "", errors.New("delete_pgbouncer action requires ActualPgBouncer")
	}

	return pgBouncer.ContainerId, nil
}

func postgresImage(version string) string {
	if version == "" {
		return "postgres:latest"
	}

	return "postgres:" + version
}

func primaryImage(cluster *ClusterSpec) string {
	if cluster.PgBackRest == nil {
		return postgresImage(cluster.Version)
	}
	if cluster.Version == "" {
		return "orca-postgres:latest"
	}
	return "orca-postgres:" + cluster.Version
}
