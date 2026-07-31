package reconciler

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

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
	failedPrimaryActions := make(map[string]struct{})
	failedDependentDeletes := make(map[string]struct{})
	failedBackupDeletes := make(map[string]struct{})
	for _, action := range actions {
		key := action.ClusterID + "\x00" + action.ReplicaID
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
	return actionType == ActionCreateReplica || actionType == ActionCreatePgBouncer || actionType == ActionUpdatePgBouncer ||
		actionType == ActionUpdateExtensions || actionType == ActionUpdatePgHba || actionType == ActionCreatePgBackRest || actionType == ActionUpdatePgBackRest
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
		return docker.StartContainer(ctx, containerID)
	case ActionCreateReplica:
		return createReplica(ctx, docker, action, desired)
	case ActionCreatePgBouncer:
		spec, err := pgBouncerContainerSpec(action)
		return createAndStart(ctx, docker, spec, err)
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
		return docker.RemoveVolume(ctx, orcadocker.VolumeName(cluster.Id))
	case ActionDeleteReplica:
		containerID, err := replicaContainerID(action)
		if err != nil {
			return err
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

type replicaDockerClient interface {
	postgres.DockerClient
	postgres.ReplicaDockerClient
	ContainerNetworkAddresses(context.Context, string) ([]string, error)
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
	addresses, err := replicaDocker.ContainerNetworkAddresses(ctx, primary)
	if err != nil {
		return fmt.Errorf("inspect primary address: %w", err)
	}
	if len(addresses) == 0 {
		return errors.New("primary container has no network address")
	}
	replicaID, err := postgres.CreateReplica(ctx, replicaDocker, postgres.ReplicaSpec{
		ClusterID:       cluster.Id,
		ReplicaID:       action.ReplicaID,
		PostgresVersion: cluster.Version,
		Primary: postgres.PrimaryConnectionInfo{
			Host: addresses[0],
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
	return nil
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
	password := make([]byte, 32)
	if _, err := rand.Read(password); err != nil {
		return fmt.Errorf("generate initial PostgreSQL password: %w", err)
	}
	spec.Env = append(spec.Env, "POSTGRES_PASSWORD="+base64.RawURLEncoding.EncodeToString(password))
	containerID, err := docker.CreateContainer(ctx, spec)
	if err != nil {
		return err
	}
	if err := docker.StartContainer(ctx, containerID); err != nil {
		return err
	}
	return markPrimaryParamsApplied(ctx, docker, containerID, spec.ClusterID, params)
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
	if err := configDocker.WriteConfig(ctx, action.ClusterID, spec.Config); err != nil {
		return fmt.Errorf("write PostgreSQL config: %w", err)
	}
	if update.Actual.Status != "running" {
		if err := docker.StartContainer(ctx, update.Actual.ContainerId); err != nil {
			return errors.Join(err, rollbackPrimaryConfig(ctx, configDocker, docker, update))
		}
		if err := markPrimaryParamsApplied(ctx, docker, update.Actual.ContainerId, action.ClusterID, update.Desired.Params); err != nil {
			return errors.Join(err, rollbackPrimaryConfig(ctx, configDocker, docker, update))
		}
		return nil
	}
	changed := postgres.ChangedParameters(update.Desired.Params, update.Actual.AppliedParams)
	rollback := func() error {
		config, renderErr := postgres.RenderConfig(action.ClusterID, update.Actual.AppliedParams)
		if renderErr != nil {
			return renderErr
		}
		return configDocker.WriteConfig(ctx, action.ClusterID, &orcadocker.ConfigMount{
			RelativePath: orcadocker.PostgresConfigRelativePath, ContainerPath: orcadocker.PostgresConfigContainerPath, Content: config,
		})
	}

	switch postgres.ClassifyConfigUpdate(changed) {
	case postgres.ConfigUpdateReload:
		slog.Info("reloading PostgreSQL configuration", "cluster_id", action.ClusterID)
		if _, err := configDocker.ExecContainer(ctx, update.Actual.ContainerId, []string{
			"psql", "--username", "postgres", "--dbname", "postgres", "--tuples-only", "--no-align", "--command", "SELECT pg_reload_conf();",
		}); err != nil {
			return errors.Join(fmt.Errorf("reload PostgreSQL config: %w", err), rollback())
		}
		if err := markPrimaryParamsApplied(ctx, docker, update.Actual.ContainerId, action.ClusterID, update.Desired.Params); err != nil {
			rollbackErr := rollback()
			_, reloadErr := configDocker.ExecContainer(ctx, update.Actual.ContainerId, []string{
				"psql", "--username", "postgres", "--dbname", "postgres", "--tuples-only", "--no-align", "--command", "SELECT pg_reload_conf();",
			})
			return errors.Join(err, rollbackErr, reloadErr)
		}
		return nil
	case postgres.ConfigUpdateRestart:
		slog.Info("restarting PostgreSQL for configuration change", "cluster_id", action.ClusterID)
		if err := docker.StopContainer(ctx, update.Actual.ContainerId); err != nil {
			return errors.Join(err, rollback())
		}
		if err := docker.StartContainer(ctx, update.Actual.ContainerId); err != nil {
			return errors.Join(err, rollbackPrimaryConfig(ctx, configDocker, docker, update))
		}
		if err := markPrimaryParamsApplied(ctx, docker, update.Actual.ContainerId, action.ClusterID, update.Desired.Params); err != nil {
			return errors.Join(err, rollbackPrimaryConfig(ctx, configDocker, docker, update))
		}
		return nil
	default:
		return errors.New("unknown PostgreSQL config update method")
	}
}

func rollbackPrimaryConfig(ctx context.Context, configDocker primaryConfigDockerClient, docker DockerClient, update *primaryUpdateSpec) error {
	config, err := postgres.RenderConfig(update.Desired.Id, update.Actual.AppliedParams)
	if err != nil {
		return err
	}
	writeErr := configDocker.WriteConfig(ctx, update.Desired.Id, &orcadocker.ConfigMount{
		RelativePath: orcadocker.PostgresConfigRelativePath, ContainerPath: orcadocker.PostgresConfigContainerPath, Content: config,
	})
	if writeErr != nil {
		return writeErr
	}
	stopErr := docker.StopContainer(ctx, update.Actual.ContainerId)
	startErr := docker.StartContainer(ctx, update.Actual.ContainerId)
	return errors.Join(stopErr, startErr)
}

func markPrimaryParamsApplied(ctx context.Context, docker DockerClient, containerID, clusterID string, params map[string]string) error {
	configDocker, ok := docker.(interface {
		WriteConfig(context.Context, string, *orcadocker.ConfigMount) error
	})
	if !ok {
		return nil
	}
	if executor, ok := docker.(interface {
		ExecContainer(context.Context, string, []string) (string, error)
	}); ok {
		if err := postgres.WaitForConfigApplied(ctx, executor, containerID, len(params)); err != nil {
			return err
		}
	}
	config, err := postgres.RenderConfig(clusterID, params)
	if err != nil {
		return err
	}
	return configDocker.WriteConfig(ctx, clusterID, &orcadocker.ConfigMount{
		RelativePath: orcadocker.PostgresAppliedConfigRelativePath,
		Content:      config,
	})
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
	return orcadocker.ContainerSpec{
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
	}, nil
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
		Image:     "pgbouncer:latest",
		Config: &orcadocker.ConfigMount{
			RelativePath:  orcadocker.PgBouncerConfigRelativePath,
			ContainerPath: orcadocker.PgBouncerConfigContainerPath,
			Content:       config,
		},
	}, nil
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
	if !ok {
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
