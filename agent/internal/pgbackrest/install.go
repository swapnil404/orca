package pgbackrest

import (
	"context"
	"fmt"

	orcadocker "github.com/swapnil404/orca/agent/internal/docker"
)

const recoveryConfigPath = "/etc/pgbackrest/pgbackrest.conf"

// InstallConfig writes the generated pgBackRest config on the primary if needed.
func InstallConfig(ctx context.Context, executor Executor, desired *ClusterDesiredState) error {
	if desired == nil {
		return fmt.Errorf("desired cluster is nil")
	}
	if executor == nil {
		return fmt.Errorf("executor is nil")
	}
	config, err := GeneratePgBackRestConfig(desired)
	if err != nil {
		return err
	}
	primary, err := primaryContainerName(desired.Id)
	if err != nil {
		return err
	}
	configPath := clusterConfigPath(desired.Id)
	command := []string{
		"sh", "-c",
		`install -d -m 0750 -o postgres -g postgres "$(dirname "$1")" "$2" && temporary="$1.orca.tmp" && printf '%s' "$3" > "$temporary" && chown postgres:postgres "$temporary" && chmod 0640 "$temporary" && if cmp -s "$temporary" "$1"; then rm -f "$temporary"; else mv "$temporary" "$1"; fi`,
		"sh", configPath, desired.PgBackRest.RepoPath, config,
	}
	if _, err := executor.ExecContainer(ctx, primary, command); err != nil {
		return fmt.Errorf("install pgBackRest config: %w", err)
	}
	return nil
}

// RemoveConfig deletes generated pgBackRest configuration from the shared cluster volume.
func RemoveConfig(ctx context.Context, executor Executor, clusterID string) error {
	if executor == nil {
		return fmt.Errorf("executor is nil")
	}
	primary, err := primaryContainerName(clusterID)
	if err != nil {
		return err
	}
	if _, err := executor.ExecContainer(ctx, primary, []string{"rm", "-f", clusterConfigPath(clusterID)}); err != nil {
		return fmt.Errorf("remove pgBackRest config: %w", err)
	}
	return nil
}

func clusterConfigPath(clusterID string) string {
	return orcadocker.VolumeMountPath(clusterID) + "/pgbackrest/pgbackrest.conf"
}
