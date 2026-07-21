package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/betterorca/betterorca/server/internal/store/sqlcdb"
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
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
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
		UserID: params.UserID,
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
	return Cluster{
		ID: cluster.ID, ProjectID: cluster.ProjectID, HostID: cluster.HostID,
		Name: cluster.Name, PostgresVersion: cluster.PostgresVersion, Parameters: parameters,
		ReplicaCount: cluster.ReplicaCount, PgBouncerEnabled: cluster.PgbouncerEnabled,
		CreatedAt: cluster.CreatedAt, UpdatedAt: cluster.UpdatedAt,
	}, nil
}

func createClusterUpsertState(ctx context.Context, queries *sqlcdb.Queries, cluster Cluster) error {
	type replica struct {
		ID string `json:"id"`
	}
	replicas := make([]replica, cluster.ReplicaCount)
	for i := range replicas {
		replicas[i].ID = fmt.Sprintf("%d", i+1)
	}
	state := struct {
		ID        string            `json:"id"`
		Version   string            `json:"version"`
		Params    map[string]string `json:"params"`
		Replicas  []replica         `json:"replicas"`
		PgBouncer map[string]string `json:"pg_bouncer,omitempty"`
	}{ID: cluster.ID, Version: cluster.PostgresVersion, Params: cluster.Parameters, Replicas: replicas}
	if cluster.PgBouncerEnabled {
		state.PgBouncer = map[string]string{"pool_mode": "transaction"}
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal cluster desired state: %w", err)
	}
	_, err = queries.CreateDesiredState(ctx, sqlcdb.CreateDesiredStateParams{
		HostID: cluster.HostID, ClusterID: cluster.ID, Operation: "upsert", State: payload,
	})
	return err
}

func desiredStateFromSQLC(state sqlcdb.DesiredState) DesiredState {
	return DesiredState{
		ID: state.ID, HostID: state.HostID, ClusterID: state.ClusterID,
		Operation: state.Operation, State: state.State, CreatedAt: state.CreatedAt,
	}
}
