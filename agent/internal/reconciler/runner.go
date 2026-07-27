package reconciler

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	orcadocker "github.com/swapnil404/orca/agent/internal/docker"
	"github.com/swapnil404/orca/agent/internal/extensions"
	"github.com/swapnil404/orca/agent/internal/hostmetrics"
	"github.com/swapnil404/orca/agent/internal/pgbackrest"
	"github.com/swapnil404/orca/agent/internal/pgbouncer"
	"github.com/swapnil404/orca/agent/internal/postgres"
	"github.com/swapnil404/orca/agent/internal/state"
	"github.com/swapnil404/orca/pkg/types"
)

// Pass contains the action outcomes and observed state from one reconciliation pass.
type Pass struct {
	Results     []ApplyResult
	Report      *types.AgentReportMessage
	acknowledge func()
}

// Acknowledge marks asynchronous results as successfully delivered.
func (p Pass) Acknowledge() {
	if p.acknowledge != nil {
		p.acknowledge()
	}
}

// Runner serializes reconciliation through the shared desired-state cache.
type Runner struct {
	cache          state.StateCache
	docker         orcadocker.DockerClient
	healthDatabase postgres.HealthDockerClient
	pgBouncer      pgbouncer.ConsoleExecutor
	hostMetrics    *hostmetrics.Collector
	extensions     extensions.PrimaryExecutor
	mu             sync.Mutex
	backups        *pgbackrest.Scheduler
	operations     *pgbackrest.OperationGate
}

type volumeLister interface {
	ListOrcaVolumes(context.Context) ([]orcadocker.VolumeInfo, error)
}

// NewRunner creates a reconciliation runner with explicit cache and Docker dependencies.
func NewRunner(cache state.StateCache, docker orcadocker.DockerClient, schedulers ...*pgbackrest.Scheduler) *Runner {
	healthDatabase, _ := docker.(postgres.HealthDockerClient)
	pgBouncerExecutor, _ := docker.(pgbouncer.ConsoleExecutor)
	extensionExecutor, _ := docker.(extensions.PrimaryExecutor)
	runner := &Runner{
		cache: cache, docker: docker, healthDatabase: healthDatabase, pgBouncer: pgBouncerExecutor,
		hostMetrics: hostmetrics.NewCollector(os.Getenv("ORCA_DATA_DIR")),
		extensions:  extensionExecutor, operations: pgbackrest.NewOperationGate(),
	}
	if len(schedulers) > 0 {
		runner.backups = schedulers[0]
		runner.backups.SetOperationGate(runner.operations)
	}
	return runner
}

// Reconcile saves a complete desired state and reconciles Docker against the cached copy.
func (r *Runner) Reconcile(ctx context.Context, desired *DesiredState) (Pass, error) {
	if err := r.operations.Acquire(ctx); err != nil {
		return Pass{}, err
	}
	defer r.operations.Release()
	r.mu.Lock()
	defer r.mu.Unlock()
	if desired == nil {
		return Pass{}, fmt.Errorf("desired state is nil")
	}

	if err := r.cache.Save(ctx, desired); err != nil {
		return Pass{}, err
	}
	return r.reconcileDesired(ctx, desired)
}

// ReconcileCached reconciles Docker against the last desired state received from the server.
func (r *Runner) ReconcileCached(ctx context.Context) (Pass, error) {
	if err := r.operations.Acquire(ctx); err != nil {
		return Pass{}, err
	}
	defer r.operations.Release()
	r.mu.Lock()
	defer r.mu.Unlock()
	desired, err := r.cache.Load(ctx)
	if err != nil {
		return Pass{}, err
	}
	return r.reconcileDesired(ctx, desired)
}

func (r *Runner) reconcileDesired(ctx context.Context, desired *DesiredState) (Pass, error) {
	containers, err := r.docker.ListOrcaContainers(ctx)
	if err != nil {
		return Pass{}, err
	}

	volumes, err := r.listVolumes(ctx)
	if err != nil {
		return Pass{}, err
	}
	actual := ActualStateFromDocker(containers, volumes)
	observationResults := r.populateInstalledExtensions(ctx, desired, actual)
	actions := Diff(desired, actual)
	results := apply(ctx, r.docker, r.backups, actions, desired)
	containers, err = r.docker.ListOrcaContainers(ctx)
	if err != nil {
		return Pass{}, err
	}
	volumes, err = r.listVolumes(ctx)
	if err != nil {
		return Pass{}, err
	}
	actual = ActualStateFromDocker(containers, volumes)
	r.ensureBackupSchedules(desired, actual)
	observationResults = append(observationResults, r.populateInstalledExtensions(ctx, desired, actual)...)
	extensionResults := apply(ctx, r.docker, r.backups, extensionOnlyActions(Diff(desired, actual)), desired)
	results = append(observationResults, results...)
	results = append(results, extensionResults...)
	results = append(results, r.populateInstalledExtensions(ctx, desired, actual)...)
	backupResults, pendingBackupResults := r.backupResults()
	results = append(results, backupResults...)
	postgres.PopulateHealth(ctx, r.healthDatabase, actual)
	pgbouncer.PopulateStatus(ctx, r.pgBouncer, actual)
	r.populateBackupSuccess(ctx, actual)
	hostMetrics, _ := r.hostMetrics.Collect()
	pass := Pass{Results: results, Report: reportFor(desired, actual, results, hostMetrics)}
	if pendingBackupResults > 0 {
		var once sync.Once
		pass.acknowledge = func() { once.Do(func() { r.backups.AcknowledgeResults(pendingBackupResults) }) }
	}
	return pass, nil
}

func (r *Runner) listVolumes(ctx context.Context) ([]orcadocker.VolumeInfo, error) {
	lister, ok := r.docker.(volumeLister)
	if !ok {
		return nil, nil
	}
	return lister.ListOrcaVolumes(ctx)
}

func (r *Runner) populateInstalledExtensions(ctx context.Context, desired *DesiredState, actual *ActualState) []ApplyResult {
	desiredByID := make(map[string]*ClusterSpec, len(desired.Clusters))
	for _, cluster := range desired.Clusters {
		if cluster != nil {
			desiredByID[cluster.Id] = cluster
		}
	}

	results := make([]ApplyResult, 0)
	for _, cluster := range actual.Clusters {
		if cluster == nil || cluster.ContainerId == "" || cluster.Status != "running" {
			continue
		}
		desiredCluster, managed := desiredByID[cluster.Id]
		if !managed {
			continue
		}
		action := Action{Type: ActionUpdateExtensions, ClusterID: cluster.Id}
		if r.extensions == nil {
			if len(desiredCluster.EnabledExtensions) > 0 {
				results = append(results, ApplyResult{
					Action: action,
					Status: ApplyStatusFailed,
					Err:    fmt.Errorf("docker client does not support extension reconciliation"),
				})
			}
			continue
		}
		installed, err := extensions.Installed(ctx, r.extensions, cluster.ContainerId)
		if err != nil {
			results = append(results, ApplyResult{Action: action, Status: ApplyStatusFailed, Err: err})
			continue
		}
		cluster.EnabledExtensions = installed
	}
	return results
}

func extensionOnlyActions(actions []Action) []Action {
	extensionActions := make([]Action, 0)
	for _, action := range actions {
		if action.Type == ActionUpdateExtensions {
			extensionActions = append(extensionActions, action)
		}
	}
	return extensionActions
}

func (r *Runner) backupResults() ([]ApplyResult, int) {
	if r.backups == nil {
		return nil, 0
	}
	queued := r.backups.PendingResults()
	results := make([]ApplyResult, 0, len(queued))
	for _, result := range queued {
		results = append(results, ApplyResult{
			Action: Action{Type: ActionRunPgBackRestBackup, ClusterID: result.ClusterID, Spec: result.BackupType},
			Status: applyStatus(result.Err), Err: result.Err,
		})
	}
	return results, len(queued)
}

func (r *Runner) populateBackupSuccess(ctx context.Context, actual *ActualState) {
	for _, cluster := range actual.Clusters {
		if cluster == nil || cluster.Backup == nil {
			continue
		}
		if unixSeconds, ok, err := pgbackrest.LastSuccessfulBackup(ctx, r.healthDatabase, cluster.ContainerId, cluster.Id); err == nil && ok {
			cluster.Backup.LastSuccessUnixSeconds = &unixSeconds
		}
	}
}

func (r *Runner) ensureBackupSchedules(desired *DesiredState, actual *ActualState) {
	if r.backups == nil {
		return
	}
	actualByID := make(map[string]*ActualCluster, len(actual.Clusters))
	for _, cluster := range actual.Clusters {
		actualByID[cluster.Id] = cluster
	}
	for _, cluster := range desired.Clusters {
		if cluster == nil || cluster.PgBackRest == nil {
			continue
		}
		state, err := pgbackrest.ReconciliationState(cluster)
		observed := actualByID[cluster.Id]
		if err == nil && observed != nil && observed.Backup != nil && observed.Backup.Config == state {
			r.backups.SetSchedule(cluster)
		}
	}
}

// ActualStateFromContainers converts Docker observations into the reconciler's actual state.
func ActualStateFromContainers(containers []orcadocker.ContainerInfo) *ActualState {
	return ActualStateFromDocker(containers, nil)
}

// ActualStateFromDocker converts Docker container and volume observations into actual state.
func ActualStateFromDocker(containers []orcadocker.ContainerInfo, volumes []orcadocker.VolumeInfo) *ActualState {
	clusters := make(map[string]*ActualCluster)
	order := make([]string, 0)
	for _, container := range containers {
		cluster, exists := clusters[container.ClusterID]
		if !exists {
			cluster = &ActualCluster{Id: container.ClusterID}
			clusters[container.ClusterID] = cluster
			order = append(order, container.ClusterID)
		}

		switch container.Kind {
		case orcadocker.ContainerKindPrimary:
			cluster.ContainerId = container.ID
			cluster.Status = container.Status
			cluster.Image = container.Image
			cluster.Version = postgresVersionFromImage(container.Image)
			cluster.AppliedParams, _ = postgres.ParseConfig(container.Config)
			if container.BackupConfig != "" {
				cluster.Backup = &types.ActualBackup{Config: container.BackupConfig}
			}
		case orcadocker.ContainerKindReplica:
			cluster.Replicas = append(cluster.Replicas, &ActualReplica{
				Id: container.ReplicaID, ContainerId: container.ID, Status: container.Status,
			})
		case orcadocker.ContainerKindPgBouncer:
			cluster.PgBouncer = &ActualPgBouncer{ContainerId: container.ID, Status: container.Status, Config: container.Config}
		}
	}
	for _, volume := range volumes {
		cluster, exists := clusters[volume.ClusterID]
		if !exists {
			cluster = &ActualCluster{Id: volume.ClusterID}
			clusters[volume.ClusterID] = cluster
			order = append(order, volume.ClusterID)
		}
		cluster.VolumeExists = true
	}

	actual := ActualState{Clusters: make([]*ActualCluster, 0, len(order))}
	for _, clusterID := range order {
		actual.Clusters = append(actual.Clusters, clusters[clusterID])
	}
	return &actual
}

func reportFor(desired *DesiredState, actual *ActualState, results []ApplyResult, hostMetrics *types.HostMetrics) *types.AgentReportMessage {
	actualByID := make(map[string]*ActualCluster, len(actual.Clusters))
	for _, cluster := range actual.Clusters {
		actualByID[cluster.Id] = cluster
	}

	health := make([]*types.ClusterHealth, 0, len(desired.Clusters)+len(actual.Clusters))
	seen := make(map[string]struct{}, len(desired.Clusters))
	for _, cluster := range desired.Clusters {
		health = append(health, &types.ClusterHealth{
			ClusterId: cluster.Id,
			Status:    clusterStatus(cluster, actualByID[cluster.Id]),
		})
		seen[cluster.Id] = struct{}{}
	}
	for _, cluster := range actual.Clusters {
		if _, exists := seen[cluster.Id]; exists {
			continue
		}
		health = append(health, &types.ClusterHealth{ClusterId: cluster.Id, Status: clusterStatus(nil, cluster)})
	}

	reconciliationResults := make([]*types.ReconciliationResult, 0, len(results))
	for _, result := range results {
		message := ""
		if result.Err != nil {
			message = result.Err.Error()
		}
		reconciliationResults = append(reconciliationResults, &types.ReconciliationResult{
			Action: string(result.Action.Type), ClusterId: result.Action.ClusterID,
			Status: string(result.Status), Error: message,
		})
	}
	return &types.AgentReportMessage{
		ActualState:           actual,
		ReconciliationResults: reconciliationResults,
		HealthReport: &types.HealthReport{
			HostMetrics: hostMetrics,
			Clusters:    health,
		},
	}
}

func clusterStatus(desired *ClusterSpec, cluster *ActualCluster) types.ClusterStatus {
	if cluster == nil || cluster.ContainerId == "" || cluster.Status != "running" || cluster.PostgresReady == nil || !cluster.GetPostgresReady() {
		return types.ClusterStatus_CLUSTER_STATUS_DOWN
	}
	replicas := make(map[string]*ActualReplica, len(cluster.Replicas))
	for _, replica := range cluster.Replicas {
		if replica == nil {
			return types.ClusterStatus_CLUSTER_STATUS_DEGRADED
		}
		replicas[replica.GetId()] = replica
		if replica.Status != "running" || replica.StandbyConnected == nil || !replica.GetStandbyConnected() || replica.StreamingState != "streaming" || replica.ReplicationLagBytes == nil || replica.ReplicationLagStatus != "known" {
			return types.ClusterStatus_CLUSTER_STATUS_DEGRADED
		}
	}
	if desired != nil {
		for _, replica := range desired.Replicas {
			if replica == nil || replicas[replica.Id] == nil {
				return types.ClusterStatus_CLUSTER_STATUS_DEGRADED
			}
		}
	}
	if cluster.PgBouncer != nil && (cluster.PgBouncer.Status != "running" || cluster.PgBouncer.AdminConsoleReachable == nil || !cluster.PgBouncer.GetAdminConsoleReachable()) {
		return types.ClusterStatus_CLUSTER_STATUS_DEGRADED
	}
	if desired != nil && desired.PgBouncer != nil && cluster.PgBouncer == nil {
		return types.ClusterStatus_CLUSTER_STATUS_DEGRADED
	}
	return types.ClusterStatus_CLUSTER_STATUS_HEALTHY
}

func postgresVersionFromImage(image string) string {
	image = strings.TrimPrefix(image, "docker.io/library/")
	version, found := strings.CutPrefix(image, "orca-postgres:")
	if !found {
		version, found = strings.CutPrefix(image, "postgres:")
	}
	if !found || version == "latest" {
		return ""
	}
	version, _, _ = strings.Cut(version, "@")
	return version
}
