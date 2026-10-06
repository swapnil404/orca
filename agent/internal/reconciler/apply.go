package reconciler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	orcadocker "github.com/swapnil404/orca/agent/internal/docker"
	"github.com/swapnil404/orca/agent/internal/pgbackrest"
	"github.com/swapnil404/orca/agent/internal/postgres"
	"sort"
	"strconv"
)

// DockerClient is the Docker wrapper interface used by reconciliation.
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

func stopAndRemove(ctx context.Context, docker DockerClient, containerID string) error {
	if containerID == "" {
		return errors.New("container ID is required")
	}
	if err := docker.StopContainer(ctx, containerID); err != nil {
		return err
	}

	return docker.RemoveContainer(ctx, containerID)
}
