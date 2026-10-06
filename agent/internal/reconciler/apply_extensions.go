package reconciler

import (
	"context"
	"errors"
	"github.com/swapnil404/orca/agent/internal/extensions"
	"github.com/swapnil404/orca/agent/internal/postgres"
	"log/slog"
)

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
