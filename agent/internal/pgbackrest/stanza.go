package pgbackrest

import (
	"context"
	"errors"
	"fmt"
	"strings"

	orcadocker "github.com/swapnil404/orca/agent/internal/docker"
)

// Executor runs pgBackRest commands against the PostgreSQL primary.
type Executor interface {
	ExecContainer(ctx context.Context, containerID string, command []string) (string, error)
}

// InitializeStanza creates the cluster stanza if it does not already exist.
func InitializeStanza(ctx context.Context, executor Executor, desired *ClusterDesiredState) error {
	if desired == nil {
		return errors.New("desired cluster is nil")
	}
	if executor == nil {
		return errors.New("executor is nil")
	}
	if _, err := GeneratePgBackRestConfig(*desired); err != nil {
		return err
	}

	primary, err := orcadocker.ContainerName(orcadocker.ContainerSpec{
		ClusterID: desired.Id,
		Kind:      orcadocker.ContainerKindPrimary,
	})
	if err != nil {
		return fmt.Errorf("resolve primary: %w", err)
	}

	if _, err := executor.ExecContainer(ctx, primary, pgBackRestCommand(desired.Id, "info")); err == nil {
		return nil
	}
	if _, err := executor.ExecContainer(ctx, primary, pgBackRestCommand(desired.Id, "stanza-create")); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "already exists") {
			return nil
		}
		return fmt.Errorf("create pgBackRest stanza %q: %w", desired.Id, err)
	}
	return nil
}

func pgBackRestCommand(stanza, operation string) []string {
	return []string{"pgbackrest", "--stanza=" + stanza, operation}
}
