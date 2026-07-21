package pgbouncer

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	orcadocker "github.com/swapnil404/orca/agent/internal/docker"
	orcatypes "github.com/swapnil404/orca/pkg/types"
)

const postgresPort = 5432

// ClusterDesiredState describes the desired state of one PostgreSQL cluster.
type ClusterDesiredState = orcatypes.ClusterSpec

// GeneratePgBouncerConfig returns the complete pgbouncer.ini for a cluster.
func GeneratePgBouncerConfig(desired ClusterDesiredState) (string, error) {
	if desired.PgBouncer == nil {
		return "", errors.New("PgBouncer settings are required")
	}
	if err := validatePgBouncerSpec(desired.PgBouncer); err != nil {
		return "", err
	}

	databases := append([]*orcatypes.DatabaseSpec(nil), desired.Databases...)
	sort.Slice(databases, func(i, j int) bool {
		return databases[i].GetName() < databases[j].GetName()
	})

	primaryHost := ""
	replicaHosts := make([]string, 0, len(desired.Replicas))
	if len(databases) > 0 {
		var err error
		primaryHost, err = containerName(desired.Id, orcadocker.ContainerKindPrimary, "")
		if err != nil {
			return "", err
		}
		for _, replica := range desired.Replicas {
			if replica == nil {
				return "", errors.New("replica is nil")
			}
			host, err := containerName(desired.Id, orcadocker.ContainerKindReplica, replica.Id)
			if err != nil {
				return "", err
			}
			replicaHosts = append(replicaHosts, host)
		}
		sort.Strings(replicaHosts)
	}

	var config strings.Builder
	config.WriteString("[databases]\n")
	previousDatabase := ""
	for _, database := range databases {
		if database == nil {
			return "", errors.New("database is nil")
		}
		name := database.Name
		if err := validateDatabaseName(name); err != nil {
			return "", err
		}
		if name == previousDatabase {
			return "", fmt.Errorf("duplicate database %q", name)
		}
		previousDatabase = name

		fmt.Fprintf(&config, "%s = host=%s port=%d dbname=%s\n", name, primaryHost, postgresPort, name)
		if len(replicaHosts) > 0 {
			fmt.Fprintf(&config, "%s_read = host=%s port=%d dbname=%s load_balance_hosts=round-robin\n", name, strings.Join(replicaHosts, ","), postgresPort, name)
		}
	}

	settings := desired.PgBouncer
	config.WriteString("\n[pgbouncer]\n")
	config.WriteString("listen_addr = 0.0.0.0\n")
	config.WriteString("listen_port = 6432\n")
	fmt.Fprintf(&config, "pool_mode = %s\n", settings.PoolMode)
	fmt.Fprintf(&config, "max_client_conn = %d\n", settings.MaxConnections)
	fmt.Fprintf(&config, "reserve_pool_size = %d\n", settings.ReservePoolSize)
	fmt.Fprintf(&config, "reserve_pool_timeout = %d\n", settings.ReservePoolTimeoutSeconds)

	return config.String(), nil
}

func validatePgBouncerSpec(spec *orcatypes.PgBouncerSpec) error {
	switch spec.PoolMode {
	case "session", "transaction", "statement":
	default:
		return fmt.Errorf("invalid pool mode %q", spec.PoolMode)
	}
	if spec.MaxConnections == 0 {
		return errors.New("max connections must be greater than zero")
	}
	if spec.ReservePoolSize > 0 && spec.ReservePoolTimeoutSeconds == 0 {
		return errors.New("reserve pool timeout must be greater than zero when reserve pool is enabled")
	}
	return nil
}

func validateDatabaseName(name string) error {
	if name == "" {
		return errors.New("database name is required")
	}
	for _, character := range name {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '_' || character == '-' {
			continue
		}
		return fmt.Errorf("database name %q contains invalid character %s", name, strconv.QuoteRune(character))
	}
	return nil
}

func containerName(clusterID string, kind orcadocker.ContainerKind, replicaID string) (string, error) {
	name, err := orcadocker.ContainerName(orcadocker.ContainerSpec{
		ClusterID: clusterID,
		Kind:      kind,
		ReplicaID: replicaID,
	})
	if err != nil {
		return "", fmt.Errorf("resolve %s host: %w", kind, err)
	}
	return name, nil
}
