package reconciler

import (
	"context"
	"strings"
	"sync"

	orcadocker "github.com/swapnil404/orca/agent/internal/docker"
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
	mu             sync.Mutex
	observers      []DesiredStateObserver
}

// DesiredStateObserver receives complete desired-state snapshots after they are cached.
type DesiredStateObserver interface {
	Update(*DesiredState)
}

// NewRunner creates a reconciliation runner with explicit cache and Docker dependencies.
func NewRunner(cache state.StateCache, docker orcadocker.DockerClient, observers ...DesiredStateObserver) *Runner {
	healthDatabase, _ := docker.(postgres.HealthDockerClient)
	return &Runner{cache: cache, docker: docker, healthDatabase: healthDatabase, observers: observers}
}

// Reconcile saves a complete desired state and reconciles Docker against the cached copy.
func (r *Runner) Reconcile(ctx context.Context, desired DesiredState) (Pass, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.cache.Save(ctx, desired); err != nil {
		return Pass{}, err
	}
	// Scheduling changes, especially removals, take effect once the snapshot is
	// durable even if Docker observation fails. A successful pass notifies again
	// so idempotent setup is retried after newly desired primaries are created.
	r.notifyObservers(&desired)
	pass, err := r.reconcileDesired(ctx, desired)
	if err == nil {
		r.notifyObservers(&desired)
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
		r.notifyObservers(&desired)
	}
	return pass, err
}

func (r *Runner) reconcileDesired(ctx context.Context, desired DesiredState) (Pass, error) {
	containers, err := r.docker.ListOrcaContainers(ctx)
	if err != nil {
		return Pass{}, err
	}

	actions := Diff(desired, ActualStateFromContainers(containers))
	results := Apply(ctx, r.docker, actions, desired)
	containers, err = r.docker.ListOrcaContainers(ctx)
	if err != nil {
		return Pass{}, err
	}
	actual := ActualStateFromContainers(containers)
	postgres.PopulateReplicaHealth(ctx, r.healthDatabase, &actual)
	return Pass{Results: results, Report: reportFor(desired, actual)}, nil
}

func (r *Runner) notifyObservers(desired *DesiredState) {
	for _, observer := range r.observers {
		if observer != nil {
			observer.Update(desired)
		}
	}
}

// ActualStateFromContainers converts Docker observations into the reconciler's actual state.
func ActualStateFromContainers(containers []orcadocker.ContainerInfo) ActualState {
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
		case orcadocker.ContainerKindReplica:
			cluster.Replicas = append(cluster.Replicas, &ActualReplica{
				Id: container.ReplicaID, ContainerId: container.ID, Status: container.Status,
			})
		case orcadocker.ContainerKindPgBouncer:
			cluster.PgBouncer = &ActualPgBouncer{ContainerId: container.ID, Status: container.Status, Config: container.Config}
		}
	}

	actual := ActualState{Clusters: make([]*ActualCluster, 0, len(order))}
	for _, clusterID := range order {
		actual.Clusters = append(actual.Clusters, clusters[clusterID])
	}
	return actual
}

func reportFor(desired DesiredState, actual ActualState) *types.AgentReportMessage {
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
		ActualState: &actual,
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
