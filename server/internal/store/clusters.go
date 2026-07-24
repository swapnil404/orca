package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/swapnil404/orca/server/internal/store/sqlcdb"
)

// Cluster is a desired Postgres cluster assigned to a host.
type Cluster struct {
	ID               string            `json:"id"`
	ProjectID        string            `json:"project_id"`
	HostID           string            `json:"host_id"`
	Name             string            `json:"name"`
	PostgresVersion  string            `json:"postgres_version"`
	Parameters       map[string]string `json:"parameters"`
	ReplicaCount     int32             `json:"replica_count"`
	PgBouncerEnabled bool              `json:"pgbouncer_enabled"`
	PgBackRest       *PgBackRestConfig `json:"pg_back_rest,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

// PgBackRestConfig contains repository, retention, and schedule settings.
type PgBackRestConfig struct {
	RepoPath            string `json:"repo_path"`
	RetentionFull       int32  `json:"retention_full"`
	RetentionDiff       int32  `json:"retention_diff"`
	FullIntervalSeconds int64  `json:"full_interval_seconds"`
	DiffIntervalSeconds int64  `json:"diff_interval_seconds"`
	IncrIntervalSeconds int64  `json:"incr_interval_seconds"`
}

type pgBackRestDesiredState struct {
	RepoPath      string                    `json:"repo_path"`
	RetentionFull int32                     `json:"retention_full"`
	RetentionDiff int32                     `json:"retention_diff"`
	Schedule      pgBackRestScheduleDesired `json:"schedule"`
}

type pgBackRestScheduleDesired struct {
	FullIntervalSeconds int64 `json:"full_interval_seconds"`
	DiffIntervalSeconds int64 `json:"diff_interval_seconds"`
	IncrIntervalSeconds int64 `json:"incr_interval_seconds"`
}

// CreateClusterParams contains the values needed to create a cluster.
type CreateClusterParams struct {
	ID               string
	UserID           string
	ProjectID        string
	HostID           string
	Name             string
	PostgresVersion  string
	Parameters       map[string]string
	ReplicaCount     int32
	PgBouncerEnabled bool
	PgBackRest       *PgBackRestConfig
}

// UpdateClusterParams contains the values needed to update a cluster.
type UpdateClusterParams struct {
	ID               string
	UserID           string
	Name             string
	PostgresVersion  string
	Parameters       map[string]string
	ReplicaCount     int32
	PgBouncerEnabled bool
	PgBackRest       *PgBackRestConfig
}

// DesiredState is one version of a cluster's desired configuration.
type DesiredState struct {
	ID        int64           `json:"id"`
	HostID    string          `json:"host_id"`
	ClusterID string          `json:"cluster_id"`
	Operation string          `json:"operation"`
	State     json.RawMessage `json:"state"`
	CreatedAt time.Time       `json:"created_at"`
}

// CreateCluster creates a cluster and its initial desired state atomically.
func (s *Postgres) CreateCluster(ctx context.Context, params CreateClusterParams) (Cluster, error) {
	parameters, err := json.Marshal(params.Parameters)
	if err != nil {
		return Cluster{}, fmt.Errorf("marshal cluster parameters: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Cluster{}, err
	}
	defer tx.Rollback()
	queries := s.queries.WithTx(tx)
	row, err := queries.CreateCluster(ctx, sqlcdb.CreateClusterParams{
		ClusterID: params.ID, ProjectID: params.ProjectID, HostID: params.HostID,
		Name: params.Name, PostgresVersion: params.PostgresVersion, Parameters: parameters,
		ReplicaCount: params.ReplicaCount, PgbouncerEnabled: params.PgBouncerEnabled,
		PgbackrestEnabled:             params.PgBackRest != nil,
		PgbackrestRepoPath:            pgBackRestRepoPath(params.PgBackRest),
		PgbackrestRetentionFull:       pgBackRestRetentionFull(params.PgBackRest),
		PgbackrestRetentionDiff:       pgBackRestRetentionDiff(params.PgBackRest),
		PgbackrestFullIntervalSeconds: pgBackRestFullInterval(params.PgBackRest),
		PgbackrestDiffIntervalSeconds: pgBackRestDiffInterval(params.PgBackRest),
		PgbackrestIncrIntervalSeconds: pgBackRestIncrInterval(params.PgBackRest),
		UserID:                        params.UserID,
	})
	if err != nil {
		return Cluster{}, err
	}
	cluster, err := clusterFromSQLC(row)
	if err != nil {
		return Cluster{}, err
	}
	if err := createClusterUpsertState(ctx, queries, cluster); err != nil {
		return Cluster{}, err
	}
	if err := tx.Commit(); err != nil {
		return Cluster{}, err
	}
	return cluster, nil
}

// ListClusters returns active clusters in an owned project.
func (s *Postgres) ListClusters(ctx context.Context, userID, projectID string) ([]Cluster, error) {
	rows, err := s.queries.ListClusters(ctx, sqlcdb.ListClustersParams{ProjectID: projectID, UserID: userID})
	if err != nil {
		return nil, err
	}
	clusters := make([]Cluster, 0, len(rows))
	for _, row := range rows {
		cluster, err := clusterFromSQLC(row)
		if err != nil {
			return nil, err
		}
		clusters = append(clusters, cluster)
	}
	return clusters, nil
}

// GetCluster returns an active cluster owned by userID.
func (s *Postgres) GetCluster(ctx context.Context, userID, clusterID string) (Cluster, error) {
	row, err := s.queries.GetCluster(ctx, sqlcdb.GetClusterParams{ID: clusterID, UserID: userID})
	if err != nil {
		return Cluster{}, err
	}
	return clusterFromSQLC(row)
}

// UpdateCluster updates a cluster and appends its desired state atomically.
func (s *Postgres) UpdateCluster(ctx context.Context, params UpdateClusterParams) (Cluster, error) {
	parameters, err := json.Marshal(params.Parameters)
	if err != nil {
		return Cluster{}, fmt.Errorf("marshal cluster parameters: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Cluster{}, err
	}
	defer tx.Rollback()
	queries := s.queries.WithTx(tx)
	row, err := queries.UpdateCluster(ctx, sqlcdb.UpdateClusterParams{
		ID: params.ID, UserID: params.UserID, Name: params.Name,
		PostgresVersion: params.PostgresVersion, Parameters: parameters,
		ReplicaCount: params.ReplicaCount, PgbouncerEnabled: params.PgBouncerEnabled,
		PgbackrestEnabled:             params.PgBackRest != nil,
		PgbackrestRepoPath:            pgBackRestRepoPath(params.PgBackRest),
		PgbackrestRetentionFull:       pgBackRestRetentionFull(params.PgBackRest),
		PgbackrestRetentionDiff:       pgBackRestRetentionDiff(params.PgBackRest),
		PgbackrestFullIntervalSeconds: pgBackRestFullInterval(params.PgBackRest),
		PgbackrestDiffIntervalSeconds: pgBackRestDiffInterval(params.PgBackRest),
		PgbackrestIncrIntervalSeconds: pgBackRestIncrInterval(params.PgBackRest),
	})
	if err != nil {
		return Cluster{}, err
	}
	cluster, err := clusterFromSQLC(row)
	if err != nil {
		return Cluster{}, err
	}
	if err := createClusterUpsertState(ctx, queries, cluster); err != nil {
		return Cluster{}, err
	}
	if err := tx.Commit(); err != nil {
		return Cluster{}, err
	}
	return cluster, nil
}

// DeleteCluster soft-deletes a cluster and appends a deletion desired state atomically.
func (s *Postgres) DeleteCluster(ctx context.Context, userID, clusterID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	queries := s.queries.WithTx(tx)
	cluster, err := queries.SoftDeleteCluster(ctx, sqlcdb.SoftDeleteClusterParams{ID: clusterID, UserID: userID})
	if err != nil {
		return err
	}
	if err := createClusterDeletionState(ctx, queries, cluster.HostID, cluster.ID); err != nil {
		return err
	}
	return tx.Commit()
}

// ListDesiredStateHistory returns all desired versions for an owned cluster.
func (s *Postgres) ListDesiredStateHistory(ctx context.Context, userID, clusterID string) ([]DesiredState, error) {
	rows, err := s.queries.ListDesiredStateHistory(ctx, sqlcdb.ListDesiredStateHistoryParams{
		ClusterID: clusterID, UserID: userID,
	})
	if err != nil {
		return nil, err
	}
	states := make([]DesiredState, 0, len(rows))
	for _, row := range rows {
		states = append(states, desiredStateFromSQLC(row))
	}
	return states, nil
}

// ListCurrentDesiredStatesForHost returns only the latest active cluster states for a host.
func (s *Postgres) ListCurrentDesiredStatesForHost(ctx context.Context, hostID string) ([]DesiredState, error) {
	rows, err := s.queries.ListCurrentDesiredStatesForHost(ctx, hostID)
	if err != nil {
		return nil, err
	}
	states := make([]DesiredState, 0, len(rows))
	for _, row := range rows {
		states = append(states, desiredStateFromSQLC(row))
	}
	return states, nil
}

func clusterFromSQLC(cluster sqlcdb.Cluster) (Cluster, error) {
	parameters := make(map[string]string)
	if err := json.Unmarshal(cluster.Parameters, &parameters); err != nil {
		return Cluster{}, fmt.Errorf("decode cluster parameters: %w", err)
	}
	result := Cluster{
		ID: cluster.ID, ProjectID: cluster.ProjectID, HostID: cluster.HostID,
		Name: cluster.Name, PostgresVersion: cluster.PostgresVersion, Parameters: parameters,
		ReplicaCount: cluster.ReplicaCount, PgBouncerEnabled: cluster.PgbouncerEnabled,
		CreatedAt: cluster.CreatedAt, UpdatedAt: cluster.UpdatedAt,
	}
	if cluster.PgbackrestEnabled {
		result.PgBackRest = &PgBackRestConfig{
			RepoPath:            cluster.PgbackrestRepoPath,
			RetentionFull:       cluster.PgbackrestRetentionFull,
			RetentionDiff:       cluster.PgbackrestRetentionDiff,
			FullIntervalSeconds: cluster.PgbackrestFullIntervalSeconds,
			DiffIntervalSeconds: cluster.PgbackrestDiffIntervalSeconds,
			IncrIntervalSeconds: cluster.PgbackrestIncrIntervalSeconds,
		}
	}
	return result, nil
}

func createClusterUpsertState(ctx context.Context, queries *sqlcdb.Queries, cluster Cluster) error {
	payload, err := clusterDesiredStatePayload(cluster)
	if err != nil {
		return err
	}
	_, err = queries.CreateDesiredState(ctx, sqlcdb.CreateDesiredStateParams{
		HostID: cluster.HostID, ClusterID: cluster.ID, Operation: "upsert", State: payload,
	})
	return err
}

func clusterDesiredStatePayload(cluster Cluster) ([]byte, error) {
	type replica struct {
		ID string `json:"id"`
	}
	replicas := make([]replica, cluster.ReplicaCount)
	for i := range replicas {
		replicas[i].ID = fmt.Sprintf("%d", i+1)
	}
	state := struct {
		ID         string                  `json:"id"`
		Version    string                  `json:"version"`
		Params     map[string]string       `json:"params"`
		Replicas   []replica               `json:"replicas"`
		PgBouncer  map[string]string       `json:"pg_bouncer,omitempty"`
		PgBackRest *pgBackRestDesiredState `json:"pg_back_rest,omitempty"`
	}{ID: cluster.ID, Version: cluster.PostgresVersion, Params: cluster.Parameters, Replicas: replicas}
	if cluster.PgBouncerEnabled {
		state.PgBouncer = map[string]string{"pool_mode": "transaction"}
	}
	if cluster.PgBackRest != nil {
		state.PgBackRest = &pgBackRestDesiredState{
			RepoPath:      cluster.PgBackRest.RepoPath,
			RetentionFull: cluster.PgBackRest.RetentionFull,
			RetentionDiff: cluster.PgBackRest.RetentionDiff,
			Schedule: pgBackRestScheduleDesired{
				FullIntervalSeconds: cluster.PgBackRest.FullIntervalSeconds,
				DiffIntervalSeconds: cluster.PgBackRest.DiffIntervalSeconds,
				IncrIntervalSeconds: cluster.PgBackRest.IncrIntervalSeconds,
			},
		}
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("marshal cluster desired state: %w", err)
	}
	return payload, nil
}

func pgBackRestRepoPath(config *PgBackRestConfig) string {
	if config == nil {
		return ""
	}
	return config.RepoPath
}

func pgBackRestRetentionFull(config *PgBackRestConfig) int32 {
	if config == nil {
		return 0
	}
	return config.RetentionFull
}

func pgBackRestRetentionDiff(config *PgBackRestConfig) int32 {
	if config == nil {
		return 0
	}
	return config.RetentionDiff
}

func pgBackRestFullInterval(config *PgBackRestConfig) int64 {
	if config == nil {
		return 0
	}
	return config.FullIntervalSeconds
}

func pgBackRestDiffInterval(config *PgBackRestConfig) int64 {
	if config == nil {
		return 0
	}
	return config.DiffIntervalSeconds
}

func pgBackRestIncrInterval(config *PgBackRestConfig) int64 {
	if config == nil {
		return 0
	}
	return config.IncrIntervalSeconds
}

func desiredStateFromSQLC(state sqlcdb.DesiredState) DesiredState {
	return DesiredState{
		ID: state.ID, HostID: state.HostID, ClusterID: state.ClusterID,
		Operation: state.Operation, State: state.State, CreatedAt: state.CreatedAt,
	}
}
