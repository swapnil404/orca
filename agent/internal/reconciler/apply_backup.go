package reconciler

import (
	"context"
	"errors"
	"fmt"
	orcadocker "github.com/swapnil404/orca/agent/internal/docker"
	"github.com/swapnil404/orca/agent/internal/pgbackrest"
)

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
