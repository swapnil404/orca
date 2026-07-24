package pgbackrest

import (
	"errors"
	"fmt"
	"strings"

	orcadocker "github.com/swapnil404/orca/agent/internal/docker"
	orcatypes "github.com/swapnil404/orca/pkg/types"
)

// ClusterDesiredState describes the desired state of one PostgreSQL cluster.
type ClusterDesiredState = orcatypes.ClusterSpec

// GeneratePgBackRestConfig returns the complete pgbackrest.conf for a cluster.
func GeneratePgBackRestConfig(desired ClusterDesiredState) (string, error) {
	if desired.PgBackRest == nil {
		return "", errors.New("pgBackRest settings are required")
	}
	if err := validateClusterID(desired.Id); err != nil {
		return "", err
	}
	if err := validateSpec(desired.PgBackRest); err != nil {
		return "", err
	}

	var config strings.Builder
	config.WriteString("[global]\n")
	fmt.Fprintf(&config, "repo1-path=%s\n", desired.PgBackRest.RepoPath)
	fmt.Fprintf(&config, "repo1-retention-full=%d\n", desired.PgBackRest.RetentionFull)
	fmt.Fprintf(&config, "repo1-retention-diff=%d\n", desired.PgBackRest.RetentionDiff)
	fmt.Fprintf(&config, "\n[%s]\n", desired.Id)
	fmt.Fprintf(&config, "pg1-path=%s/primary\n", orcadocker.VolumeMountPath(desired.Id))

	return config.String(), nil
}

func validateSpec(spec *orcatypes.PgBackRestSpec) error {
	if spec.RepoPath == "" {
		return errors.New("repository path is required")
	}
	if strings.ContainsAny(spec.RepoPath, "\r\n") {
		return errors.New("repository path must not contain a newline")
	}
	if spec.RetentionFull == 0 {
		return errors.New("full retention must be greater than zero")
	}
	if spec.RetentionDiff == 0 {
		return errors.New("differential retention must be greater than zero")
	}
	return nil
}

func validateClusterID(clusterID string) error {
	if clusterID == "" {
		return errors.New("cluster ID is required")
	}
	for _, character := range clusterID {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '-' || character == '_' {
			continue
		}
		return fmt.Errorf("cluster ID %q contains invalid character %q", clusterID, character)
	}
	return nil
}
