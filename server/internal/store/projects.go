package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/betterorca/betterorca/server/internal/store/sqlcdb"
)

// Project groups clusters owned by one user.
type Project struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateProjectParams contains the values needed to create a project.
type CreateProjectParams struct {
	ID     string
	UserID string
	Name   string
}

// UpdateProjectParams contains the values needed to update a project.
type UpdateProjectParams struct {
	ID     string
	UserID string
	Name   string
}

// CreateProject persists a project for a user.
func (s *Postgres) CreateProject(ctx context.Context, params CreateProjectParams) (Project, error) {
	project, err := s.queries.CreateProject(ctx, sqlcdb.CreateProjectParams{
		ID: params.ID, UserID: params.UserID, Name: params.Name,
	})
	if err != nil {
		return Project{}, err
	}
	return projectFromSQLC(project), nil
}

// ListProjects returns active projects owned by userID.
func (s *Postgres) ListProjects(ctx context.Context, userID string) ([]Project, error) {
	rows, err := s.queries.ListProjects(ctx, userID)
	if err != nil {
		return nil, err
	}
	projects := make([]Project, 0, len(rows))
	for _, row := range rows {
		projects = append(projects, projectFromSQLC(row))
	}
	return projects, nil
}

// ListProjectIDsForHost returns active projects containing clusters assigned to hostID.
func (s *Postgres) ListProjectIDsForHost(ctx context.Context, hostID string) ([]string, error) {
	return s.queries.ListProjectIDsForHost(ctx, hostID)
}

// GetProject returns an active project owned by userID.
func (s *Postgres) GetProject(ctx context.Context, userID, projectID string) (Project, error) {
	project, err := s.queries.GetProject(ctx, sqlcdb.GetProjectParams{ID: projectID, UserID: userID})
	if err != nil {
		return Project{}, err
	}
	return projectFromSQLC(project), nil
}

// UpdateProject updates an active project owned by the user.
func (s *Postgres) UpdateProject(ctx context.Context, params UpdateProjectParams) (Project, error) {
	project, err := s.queries.UpdateProject(ctx, sqlcdb.UpdateProjectParams{
		ID: params.ID, UserID: params.UserID, Name: params.Name,
	})
	if err != nil {
		return Project{}, err
	}
	return projectFromSQLC(project), nil
}

// DeleteProject soft-deletes a project and records deletion states for all its clusters.
func (s *Postgres) DeleteProject(ctx context.Context, userID, projectID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	queries := s.queries.WithTx(tx)

	clusters, err := queries.ListActiveClustersForProject(ctx, sqlcdb.ListActiveClustersForProjectParams{
		ProjectID: projectID, UserID: userID,
	})
	if err != nil {
		return err
	}
	if err := queries.SoftDeleteClustersForProject(ctx, sqlcdb.SoftDeleteClustersForProjectParams{
		ProjectID: projectID, UserID: userID,
	}); err != nil {
		return err
	}
	for _, cluster := range clusters {
		if err := createClusterDeletionState(ctx, queries, cluster.HostID, cluster.ID); err != nil {
			return err
		}
	}
	if _, err := queries.SoftDeleteProject(ctx, sqlcdb.SoftDeleteProjectParams{ID: projectID, UserID: userID}); err != nil {
		return err
	}
	return tx.Commit()
}

func projectFromSQLC(project sqlcdb.Project) Project {
	return Project{ID: project.ID, Name: project.Name, CreatedAt: project.CreatedAt, UpdatedAt: project.UpdatedAt}
}

func createClusterDeletionState(ctx context.Context, queries *sqlcdb.Queries, hostID, clusterID string) error {
	state, err := json.Marshal(struct {
		ClusterID string `json:"cluster_id"`
		Exists    bool   `json:"exists"`
	}{ClusterID: clusterID, Exists: false})
	if err != nil {
		return fmt.Errorf("marshal cluster deletion state: %w", err)
	}
	_, err = queries.CreateDesiredState(ctx, sqlcdb.CreateDesiredStateParams{
		HostID: hostID, ClusterID: clusterID, Operation: "delete", State: state,
	})
	return err
}
