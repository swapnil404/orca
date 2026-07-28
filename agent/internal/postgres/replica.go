package postgres

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	orcadocker "github.com/swapnil404/orca/agent/internal/docker"
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
	Primary         PrimaryConnectionInfo
	PostgresVersion string
}

// ReplicaIdentity contains every resource name derived from a stable replica ID.
type ReplicaIdentity struct {
	ContainerName string
	DataPath      string
	SlotName      string
}

// DeriveReplicaIdentity derives all persistent replica resource names from one stable ID.
func DeriveReplicaIdentity(clusterID, replicaID string) (ReplicaIdentity, error) {
	if replicaID == "" || filepath.Base(replicaID) != replicaID || replicaID == "." || replicaID == ".." {
		return ReplicaIdentity{}, errors.New("replica ID must be a non-empty path segment")
	}
	containerName, err := orcadocker.ContainerName(orcadocker.ContainerSpec{
		ClusterID: clusterID,
		Kind:      orcadocker.ContainerKindReplica,
		ReplicaID: replicaID,
	})
	if err != nil {
		return ReplicaIdentity{}, err
	}
	return ReplicaIdentity{
		ContainerName: containerName,
		DataPath:      fmt.Sprintf("%s/replicas/%s", orcadocker.VolumeMountPath(clusterID), replicaID),
		SlotName:      "replica_" + hex.EncodeToString([]byte(replicaID)),
	}, nil
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
	ReplicaID  string
	Err        error
	CleanupErr error
}

// Error returns a readable replica bootstrap failure.
func (e *ReplicaBootstrapError) Error() string {
	message := fmt.Sprintf("bootstrap replica %q for cluster %q with pg_basebackup: %v", e.ReplicaID, e.ClusterID, e.Err)
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

	identity, err := DeriveReplicaIdentity(spec.ClusterID, spec.ReplicaID)
	if err != nil {
		return "", err
	}
	dataPath := identity.DataPath
	bootstrapSpec, err := replicaContainerSpec(spec, identity, true)
	if err != nil {
		return "", err
	}
	bootstrapID, err := docker.CreateContainer(ctx, bootstrapSpec)
	if err != nil {
		return "", fmt.Errorf("create replica bootstrap container: %w", err)
	}
	if err := docker.StartContainer(ctx, bootstrapID); err != nil {
		cleanupErr := docker.RemoveContainer(ctx, bootstrapID)
		return "", errors.Join(fmt.Errorf("start replica bootstrap container: %w", err), cleanupErr)
	}

	if _, err := docker.ExecContainer(ctx, bootstrapID, prepareReplicaDataCommand(dataPath)); err != nil {
		cleanupErr := cleanupBootstrap(ctx, docker, bootstrapID, spec.ClusterID, identity)
		return "", errors.Join(fmt.Errorf("prepare replica data directory: %w", err), cleanupErr)
	}
	if _, err := docker.ExecContainer(ctx, bootstrapID, baseBackupCommand(spec, identity)); err != nil {
		return "", &ReplicaBootstrapError{
			ClusterID:  spec.ClusterID,
			ReplicaID:  spec.ReplicaID,
			Err:        err,
			CleanupErr: cleanupBootstrap(ctx, docker, bootstrapID, spec.ClusterID, identity),
		}
	}
	if _, err := docker.ExecContainer(ctx, bootstrapID, writeRecoveryConfigCommand(dataPath, recoveryConfig(spec, identity))); err != nil {
		cleanupErr := cleanupBootstrap(ctx, docker, bootstrapID, spec.ClusterID, identity)
		return "", errors.Join(fmt.Errorf("write replica recovery config: %w", err), cleanupErr)
	}
	if err := removeContainer(ctx, docker, bootstrapID); err != nil {
		return "", fmt.Errorf("remove replica bootstrap container: %w", err)
	}

	replicaSpec, err := replicaContainerSpec(spec, identity, false)
	if err != nil {
		return "", err
	}
	replicaID, err := docker.CreateContainer(ctx, replicaSpec)
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

func replicaContainerSpec(spec ReplicaSpec, identity ReplicaIdentity, bootstrap bool) (orcadocker.ContainerSpec, error) {
	replicaID := spec.ReplicaID
	command := []string(nil)
	if bootstrap {
		replicaID += bootstrapSuffix
		command = []string{"sleep", "infinity"}
	}

	containerSpec := orcadocker.ContainerSpec{
		ClusterID: spec.ClusterID,
		Kind:      orcadocker.ContainerKindReplica,
		ReplicaID: replicaID,
		Image:     postgresImageForVersion(spec.PostgresVersion),
		Env: []string{
			"POSTGRES_HOST_AUTH_METHOD=trust",
			"PGDATA=" + identity.DataPath,
		},
		Command:   command,
		UseVolume: true,
	}
	name, err := orcadocker.ContainerName(containerSpec)
	if err != nil {
		return orcadocker.ContainerSpec{}, err
	}
	expectedName := identity.ContainerName
	if bootstrap {
		expectedName += bootstrapSuffix
	}
	if name != expectedName {
		return orcadocker.ContainerSpec{}, errors.New("replica container identity is inconsistent")
	}
	return containerSpec, nil
}

func postgresImageForVersion(version string) string {
	if version == "" {
		return defaultPostgresImage
	}
	return "postgres:" + version
}

func prepareReplicaDataCommand(dataPath string) []string {
	return []string{
		"sh", "-c",
		`rm -rf -- "$1" && install -d -m 0700 -o postgres -g postgres "$1"`,
		"sh", dataPath,
	}
}

func baseBackupCommand(spec ReplicaSpec, identity ReplicaIdentity) []string {
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
		"--pgdata", identity.DataPath,
		"--slot", identity.SlotName,
		"--wal-method", "stream",
		"--checkpoint", "fast",
		"--no-password",
	}
}

func recoveryConfig(spec ReplicaSpec, identity ReplicaIdentity) string {
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
		quotePostgresConfig(identity.SlotName),
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

func cleanupBootstrap(ctx context.Context, docker ReplicaDockerClient, containerID, clusterID string, identity ReplicaIdentity) error {
	_, dataErr := docker.ExecContainer(ctx, containerID, []string{"rm", "-rf", "--", identity.DataPath})
	containerErr := removeContainer(ctx, docker, containerID)
	primary, nameErr := orcadocker.ContainerName(orcadocker.ContainerSpec{
		ClusterID: clusterID,
		Kind:      orcadocker.ContainerKindPrimary,
	})
	if nameErr != nil {
		return errors.Join(dataErr, containerErr, nameErr)
	}
	return errors.Join(dataErr, containerErr, dropReplicationSlot(ctx, docker, primary, identity.SlotName))
}

func removeContainer(ctx context.Context, docker ReplicaDockerClient, containerID string) error {
	stopErr := docker.StopContainer(ctx, containerID)
	removeErr := docker.RemoveContainer(ctx, containerID)
	return errors.Join(stopErr, removeErr)
}
