package pgbackrest

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	orcadocker "github.com/swapnil404/orca/agent/internal/docker"
	orcatypes "github.com/swapnil404/orca/pkg/types"
)

// ClusterDesiredState describes the desired state of one PostgreSQL cluster.
type ClusterDesiredState = orcatypes.ClusterSpec

// GeneratePgBackRestConfig returns the complete pgbackrest.conf for a cluster.
func GeneratePgBackRestConfig(desired *ClusterDesiredState) (string, error) {
	if desired == nil {
		return "", errors.New("desired cluster is nil")
	}
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

// ReconciliationState returns the persisted value used to compare desired and applied backup configuration.
func ReconciliationState(desired *ClusterDesiredState) (string, error) {
	config, err := GeneratePgBackRestConfig(desired)
	if err != nil {
		return "", err
	}
	schedule := desired.PgBackRest.Schedule
	if schedule == nil {
		return config + "\n[orca-schedule]\n\n" + repositoryMountState(desired.PgBackRest.RepoPath), nil
	}
	return fmt.Sprintf("%s\n[orca-schedule]\nfull=%d\ndiff=%d\nincr=%d\n\n%s", config,
		schedule.FullIntervalSeconds, schedule.DiffIntervalSeconds, schedule.IncrIntervalSeconds, repositoryMountState(desired.PgBackRest.RepoPath)), nil
}

func repositoryMountState(repoPath string) string {
	return fmt.Sprintf("[orca-storage]\nrepo-bind=%s\n", filepath.Clean(repoPath))
}

func validateSpec(spec *orcatypes.PgBackRestSpec) error {
	if spec.RepoPath == "" {
		return errors.New("repository path is required")
	}
	if strings.ContainsAny(spec.RepoPath, "\r\n") {
		return errors.New("repository path must not contain a newline")
	}
	if !filepath.IsAbs(spec.RepoPath) || filepath.Clean(spec.RepoPath) == string(filepath.Separator) {
		return errors.New("repository path must be an absolute host directory other than the filesystem root")
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
