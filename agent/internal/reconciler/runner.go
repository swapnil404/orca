package reconciler

import (
	"context"
	"fmt"
	"strings"
	"sync"

	orcadocker "github.com/swapnil404/orca/agent/internal/docker"
	"github.com/swapnil404/orca/agent/internal/extensions"
	"github.com/swapnil404/orca/agent/internal/postgres"
	"github.com/swapnil404/orca/agent/internal/state"
	"github.com/swapnil404/orca/pkg/types"
)

// Pass contains the action outcomes and observed state from one reconciliation pass.
type Pass struct {
	Results []ApplyResult
	Report  *types.AgentReportMessage
}

// Runner serializes reconciliation through the shared desired-state cache.
type Runner struct {
	cache          state.StateCache
	docker         orcadocker.DockerClient
	healthDatabase postgres.HealthDockerClient
	extensions     extensions.PrimaryExecutor
	mu             sync.Mutex
	observers      []DesiredStateObserver
}

// DesiredStateObserver receives complete desired-state snapshots after they are cached.
type DesiredStateObserver interface {
	Update(*DesiredState)
}

type volumeLister interface {
	ListOrcaVolumes(context.Context) ([]orcadocker.VolumeInfo, error)
}

// NewRunner creates a reconciliation runner with explicit cache and Docker dependencies.
func NewRunner(cache state.StateCache, docker orcadocker.DockerClient, observers ...DesiredStateObserver) *Runner {
	healthDatabase, _ := docker.(postgres.HealthDockerClient)
	extensionExecutor, _ := docker.(extensions.PrimaryExecutor)
	return &Runner{
		cache: cache, docker: docker, healthDatabase: healthDatabase,
		extensions: extensionExecutor, observers: observers,
	}
}

// Reconcile saves a complete desired state and reconciles Docker against the cached copy.
func (r *Runner) Reconcile(ctx context.Context, desired *DesiredState) (Pass, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if desired == nil {
		return Pass{}, fmt.Errorf("desired state is nil")
	}

	if err := r.cache.Save(ctx, desired); err != nil {
		return Pass{}, err
	}
	// Scheduling changes, especially removals, take effect once the snapshot is
	// durable even if Docker observation fails. A successful pass notifies again
	// so idempotent setup is retried after newly desired primaries are created.
	r.notifyObservers(desired)
	pass, err := r.reconcileDesired(ctx, desired)
	if err == nil {
		r.notifyObservers(desired)
	}
	return pass, err
}

// ReconcileCached reconciles Docker against the last desired state received from the server.
func (r *Runner) ReconcileCached(ctx context.Context) (Pass, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	desired, err := r.cache.Load(ctx)
	if err != nil {
		return Pass{}, err
	}
	pass, err := r.reconcileDesired(ctx, desired)
	if err == nil {
		r.notifyObservers(desired)
	}
	return pass, err
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
	results := Apply(ctx, r.docker, actions, desired)
	containers, err = r.docker.ListOrcaContainers(ctx)
	if err != nil {
		return Pass{}, err
	}
	volumes, err = r.listVolumes(ctx)
	if err != nil {
		return Pass{}, err
	}
	actual = ActualStateFromDocker(containers, volumes)
	observationResults = append(observationResults, r.populateInstalledExtensions(ctx, desired, actual)...)
	extensionResults := Apply(ctx, r.docker, extensionOnlyActions(Diff(desired, actual)), desired)
	results = append(observationResults, results...)
	results = append(results, extensionResults...)
	results = append(results, r.populateInstalledExtensions(ctx, desired, actual)...)
	postgres.PopulateReplicaHealth(ctx, r.healthDatabase, actual)
	return Pass{Results: results, Report: reportFor(desired, actual)}, nil
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
					Err:    fmt.Errorf("docker client does not support extension reconciliation"),
				})
			}
			continue
		}
		installed, err := extensions.Installed(ctx, r.extensions, cluster.ContainerId)
		if err != nil {
			results = append(results, ApplyResult{Action: action, Err: err})
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

func (r *Runner) notifyObservers(desired *DesiredState) {
	for _, observer := range r.observers {
		if observer != nil {
			observer.Update(desired)
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
			cluster.Version = postgresVersionFromImage(container.Image)
			cluster.AppliedParams, _ = postgres.ParseConfig(container.Config)
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

func reportFor(desired *DesiredState, actual *ActualState) *types.AgentReportMessage {
	actualByID := make(map[string]*ActualCluster, len(actual.Clusters))
	for _, cluster := range actual.Clusters {
		actualByID[cluster.Id] = cluster
	}

	health := make([]*types.ClusterHealth, 0, len(desired.Clusters)+len(actual.Clusters))
	seen := make(map[string]struct{}, len(desired.Clusters))
	for _, cluster := range desired.Clusters {
		health = append(health, &types.ClusterHealth{
			ClusterId: cluster.Id,
			Status:    clusterStatus(actualByID[cluster.Id]),
		})
		seen[cluster.Id] = struct{}{}
	}
	for _, cluster := range actual.Clusters {
		if _, exists := seen[cluster.Id]; exists {
			continue
		}
		health = append(health, &types.ClusterHealth{ClusterId: cluster.Id, Status: clusterStatus(cluster)})
	}

	return &types.AgentReportMessage{
		ActualState: actual,
		HealthReport: &types.HealthReport{
			HostMetrics: &types.HostMetrics{},
			Clusters:    health,
		},
	}
}

func clusterStatus(cluster *ActualCluster) types.ClusterStatus {
	if cluster == nil || cluster.ContainerId == "" || cluster.Status != "running" {
		return types.ClusterStatus_CLUSTER_STATUS_DOWN
	}
	for _, replica := range cluster.Replicas {
		if replica.Status != "running" {
			return types.ClusterStatus_CLUSTER_STATUS_DEGRADED
		}
	}
	if cluster.PgBouncer != nil && cluster.PgBouncer.Status != "running" {
		return types.ClusterStatus_CLUSTER_STATUS_DEGRADED
	}
	return types.ClusterStatus_CLUSTER_STATUS_HEALTHY
}

func postgresVersionFromImage(image string) string {
	image = strings.TrimPrefix(image, "docker.io/library/")
	version, found := strings.CutPrefix(image, "postgres:")
	if !found || version == "latest" {
		return ""
	}
	version, _, _ = strings.Cut(version, "@")
	return version
}
