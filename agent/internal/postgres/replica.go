package postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	orcadocker "github.com/betterorca/betterorca/agent/internal/docker"
)

const (
	defaultPostgresImage = "postgres:latest"
	defaultPostgresPort  = 5432
	bootstrapSuffix      = "-bootstrap"
	bootstrapMarker      = ".orca-bootstrap-complete"
)

// PrimaryConnectionInfo identifies the primary used to bootstrap a replica.
type PrimaryConnectionInfo struct {
	Host string
	Port int
	User string
}

// ReplicaSpec describes a PostgreSQL replica to create.
type ReplicaSpec struct {
	ClusterID       string
	ReplicaID       string
	Index           int
	Primary         PrimaryConnectionInfo
	PostgresVersion string
}

// ReplicaDockerClient is the Docker functionality required to create a replica.
type ReplicaDockerClient interface {
	CreateContainer(ctx context.Context, spec orcadocker.ContainerSpec) (containerID string, err error)
	StartContainer(ctx context.Context, containerID string) error
	StopContainer(ctx context.Context, containerID string) error
	RemoveContainer(ctx context.Context, containerID string) error
	ExecContainer(ctx context.Context, containerID string, command []string) (string, error)
}

// ReplicaBootstrapError reports a failed pg_basebackup and any cleanup failure.
type ReplicaBootstrapError struct {
	ClusterID  string
	Index      int
	Err        error
	CleanupErr error
}

// Error returns a readable replica bootstrap failure.
func (e *ReplicaBootstrapError) Error() string {
	message := fmt.Sprintf("bootstrap replica %d for cluster %q with pg_basebackup: %v", e.Index, e.ClusterID, e.Err)
	if e.CleanupErr != nil {
		message += fmt.Sprintf("; cleanup failed: %v", e.CleanupErr)
	}
	return message
}

// Unwrap returns the pg_basebackup failure.
func (e *ReplicaBootstrapError) Unwrap() error {
	return e.Err
}

// CreateReplica bootstraps and starts a PostgreSQL streaming replica.
func CreateReplica(ctx context.Context, docker ReplicaDockerClient, spec ReplicaSpec) (string, error) {
	if docker == nil {
		return "", errors.New("docker client is nil")
	}
	if err := validateReplicaSpec(spec); err != nil {
		return "", err
	}

	dataPath := replicaDataPath(spec)
	bootstrapSpec := replicaContainerSpec(spec, true)
	bootstrapID, err := docker.CreateContainer(ctx, bootstrapSpec)
	if err != nil {
		return "", fmt.Errorf("create replica bootstrap container: %w", err)
	}
	if err := docker.StartContainer(ctx, bootstrapID); err != nil {
		cleanupErr := docker.RemoveContainer(ctx, bootstrapID)
		return "", errors.Join(fmt.Errorf("start replica bootstrap container: %w", err), cleanupErr)
	}

	if _, err := docker.ExecContainer(ctx, bootstrapID, prepareReplicaDataCommand(dataPath)); err != nil {
		cleanupErr := cleanupBootstrap(ctx, docker, bootstrapID, dataPath)
		return "", errors.Join(fmt.Errorf("prepare replica data directory: %w", err), cleanupErr)
	}
	if _, err := docker.ExecContainer(ctx, bootstrapID, baseBackupCommand(spec)); err != nil {
		return "", &ReplicaBootstrapError{
			ClusterID:  spec.ClusterID,
			Index:      spec.Index,
			Err:        err,
			CleanupErr: cleanupBootstrap(ctx, docker, bootstrapID, dataPath),
		}
	}
	if _, err := docker.ExecContainer(ctx, bootstrapID, writeRecoveryConfigCommand(dataPath, recoveryConfig(spec))); err != nil {
		cleanupErr := cleanupBootstrap(ctx, docker, bootstrapID, dataPath)
		return "", errors.Join(fmt.Errorf("write replica recovery config: %w", err), cleanupErr)
	}
	if err := removeContainer(ctx, docker, bootstrapID); err != nil {
		return "", fmt.Errorf("remove replica bootstrap container: %w", err)
	}

	replicaID, err := docker.CreateContainer(ctx, replicaContainerSpec(spec, false))
	if err != nil {
		return "", fmt.Errorf("create replica container: %w", err)
	}
	if err := docker.StartContainer(ctx, replicaID); err != nil {
		cleanupErr := docker.RemoveContainer(ctx, replicaID)
		return "", errors.Join(fmt.Errorf("start replica container: %w", err), cleanupErr)
	}

	return replicaID, nil
}

func validateReplicaSpec(spec ReplicaSpec) error {
	if spec.ClusterID == "" {
		return errors.New("cluster ID is required")
	}
	if spec.Index < 1 {
		return errors.New("replica index must be greater than zero")
	}
	if spec.ReplicaID == "" {
		return errors.New("replica ID is required")
	}
	if spec.Primary.Host == "" {
		return errors.New("primary host is required")
	}
	if spec.Primary.Port < 0 || spec.Primary.Port > 65535 {
		return errors.New("primary port must be zero or between 1 and 65535")
	}
	return nil
}

func replicaContainerSpec(spec ReplicaSpec, bootstrap bool) orcadocker.ContainerSpec {
	replicaID := spec.ReplicaID
	command := []string(nil)
	if bootstrap {
		replicaID += bootstrapSuffix
		command = []string{"sleep", "infinity"}
	}

	return orcadocker.ContainerSpec{
		ClusterID: spec.ClusterID,
		Kind:      orcadocker.ContainerKindReplica,
		ReplicaID: replicaID,
		Image:     postgresImageForVersion(spec.PostgresVersion),
		Env: []string{
			"POSTGRES_HOST_AUTH_METHOD=trust",
			"PGDATA=" + replicaDataPath(spec),
		},
		Command:   command,
		UseVolume: true,
	}
}

func postgresImageForVersion(version string) string {
	if version == "" {
		return defaultPostgresImage
	}
	return "postgres:" + version
}

func replicaDataPath(spec ReplicaSpec) string {
	return fmt.Sprintf("%s/replica-%d", orcadocker.VolumeMountPath(spec.ClusterID), spec.Index)
}

func replicaSlotName(index int) string {
	return fmt.Sprintf("replica_%d", index)
}

func prepareReplicaDataCommand(dataPath string) []string {
	return []string{
		"sh", "-c",
		`rm -rf -- "$1" && install -d -m 0700 -o postgres -g postgres "$1"`,
		"sh", dataPath,
	}
}

func baseBackupCommand(spec ReplicaSpec) []string {
	port := spec.Primary.Port
	if port == 0 {
		port = defaultPostgresPort
	}
	user := spec.Primary.User
	if user == "" {
		user = postgresUser
	}

	return []string{
		"gosu", postgresUser,
		"pg_basebackup",
		"--host", spec.Primary.Host,
		"--port", strconv.Itoa(port),
		"--username", user,
		"--pgdata", replicaDataPath(spec),
		"--slot", replicaSlotName(spec.Index),
		"--wal-method", "stream",
		"--checkpoint", "fast",
		"--no-password",
	}
}

func recoveryConfig(spec ReplicaSpec) string {
	port := spec.Primary.Port
	if port == 0 {
		port = defaultPostgresPort
	}
	user := spec.Primary.User
	if user == "" {
		user = postgresUser
	}
	conninfo := fmt.Sprintf("host=%s port=%d user=%s", quoteConninfo(spec.Primary.Host), port, quoteConninfo(user))

	return fmt.Sprintf(
		"primary_conninfo = %s\nprimary_slot_name = %s\n",
		quotePostgresConfig(conninfo),
		quotePostgresConfig(replicaSlotName(spec.Index)),
	)
}

func quoteConninfo(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `'`, `\'`)
	return "'" + value + "'"
}

func quotePostgresConfig(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func writeRecoveryConfigCommand(dataPath, config string) []string {
	return []string{
		"sh", "-c",
		`printf '\n%s' "$2" >> "$1/postgresql.auto.conf" && : > "$1/standby.signal" && chown postgres:postgres "$1/postgresql.auto.conf" "$1/standby.signal" && : > "$1/` + bootstrapMarker + `"`,
		"sh", dataPath, config,
	}
}

func cleanupBootstrap(ctx context.Context, docker ReplicaDockerClient, containerID, dataPath string) error {
	_, dataErr := docker.ExecContainer(ctx, containerID, []string{"rm", "-rf", "--", dataPath})
	return errors.Join(dataErr, removeContainer(ctx, docker, containerID))
}

func removeContainer(ctx context.Context, docker ReplicaDockerClient, containerID string) error {
	stopErr := docker.StopContainer(ctx, containerID)
	removeErr := docker.RemoveContainer(ctx, containerID)
	return errors.Join(stopErr, removeErr)
}
