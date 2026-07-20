package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/betterorca/betterorca/pkg/types"
	"github.com/betterorca/betterorca/server/internal/store"
)

const maxFrontendMessageBytes = 64 * 1024

type projectEventStore interface {
	GetProject(context.Context, string, string) (store.Project, error)
	ListClusters(context.Context, string, string) ([]store.Cluster, error)
	ListClusterReportsForHost(context.Context, string, time.Time) ([]store.ClusterReport, error)
	ListProjectIDsForHost(context.Context, string) ([]string, error)
}

// ProjectStateSnapshot is the current actual state and health for a project.
type ProjectStateSnapshot struct {
	Type      string                `json:"type"`
	ProjectID string                `json:"project_id"`
	Clusters  []ProjectClusterState `json:"clusters"`
}

// ProjectClusterState is the latest reported state for one desired cluster.
type ProjectClusterState struct {
	ClusterID   string               `json:"cluster_id"`
	HostID      string               `json:"host_id"`
	ActualState *types.ActualCluster `json:"actual_state"`
	Health      string               `json:"health"`
	LastSeen    *time.Time           `json:"last_seen,omitempty"`
	Stale       bool                 `json:"stale"`
}

type projectClient struct {
	connection *websocket.Conn
	userID     string
}

// ProjectEventHandler serves project-scoped frontend WebSocket subscriptions.
type ProjectEventHandler struct {
	store         projectEventStore
	now           func() time.Time
	upgrader      websocket.Upgrader
	mu            sync.Mutex
	subscriptions map[string]map[*projectClient]struct{}
}

// NewProjectEventHandler creates a frontend WebSocket endpoint and report notifier.
func NewProjectEventHandler(state projectEventStore) *ProjectEventHandler {
	return &ProjectEventHandler{
		store:         state,
		now:           time.Now,
		subscriptions: make(map[string]map[*projectClient]struct{}),
	}
}

// RegisterRoutes registers the project event WebSocket route on mux.
func (h *ProjectEventHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /projects/{projectID}/events", h)
}

// ServeHTTP authenticates, scopes, and upgrades a frontend project subscription.
func (h *ProjectEventHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	projectID := r.PathValue("projectID")
	if _, err := h.store.GetProject(r.Context(), userID, projectID); err != nil {
		writeStoreError(w, err)
		return
	}

	connection, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	connection.SetReadLimit(maxFrontendMessageBytes)
	client := &projectClient{connection: connection, userID: userID}
	if err := h.subscribe(r.Context(), userID, projectID, client); err != nil {
		_ = connection.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "failed to load project state"),
			time.Now().Add(time.Second))
		_ = connection.Close()
		return
	}
	defer h.unsubscribe(projectID, client)

	for {
		if _, _, err := connection.ReadMessage(); err != nil {
			return
		}
	}
}

// NotifyHostReport publishes fresh snapshots after a host report is committed.
func (h *ProjectEventHandler) NotifyHostReport(ctx context.Context, hostID string) error {
	projectIDs, err := h.store.ListProjectIDsForHost(ctx, hostID)
	if err != nil {
		return err
	}
	var publishErr error
	for _, projectID := range projectIDs {
		if err := h.publish(ctx, projectID); err != nil {
			publishErr = errors.Join(publishErr, err)
		}
	}
	return publishErr
}

func (h *ProjectEventHandler) subscribe(ctx context.Context, userID, projectID string, client *projectClient) error {
	// Snapshot loading and registration are atomic with publication: a report is
	// either represented by this snapshot or delivered immediately afterward.
	h.mu.Lock()
	defer h.mu.Unlock()

	snapshot, err := h.snapshot(ctx, userID, projectID)
	if err != nil {
		return err
	}
	if err := client.connection.WriteJSON(snapshot); err != nil {
		return err
	}
	if h.subscriptions[projectID] == nil {
		h.subscriptions[projectID] = make(map[*projectClient]struct{})
	}
	h.subscriptions[projectID][client] = struct{}{}
	return nil
}

func (h *ProjectEventHandler) publish(ctx context.Context, projectID string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	clients := h.subscriptions[projectID]
	if len(clients) == 0 {
		return nil
	}
	var userID string
	for client := range clients {
		userID = client.userID
		break
	}
	snapshot, err := h.snapshot(ctx, userID, projectID)
	if err != nil {
		return err
	}
	for client := range clients {
		if err := client.connection.WriteJSON(snapshot); err != nil {
			delete(clients, client)
			_ = client.connection.Close()
		}
	}
	if len(clients) == 0 {
		delete(h.subscriptions, projectID)
	}
	return nil
}

func (h *ProjectEventHandler) unsubscribe(projectID string, client *projectClient) {
	h.mu.Lock()
	clients := h.subscriptions[projectID]
	delete(clients, client)
	if len(clients) == 0 {
		delete(h.subscriptions, projectID)
	}
	h.mu.Unlock()
	_ = client.connection.Close()
}

func (h *ProjectEventHandler) snapshot(ctx context.Context, userID, projectID string) (ProjectStateSnapshot, error) {
	clusters, err := h.store.ListClusters(ctx, userID, projectID)
	if err != nil {
		return ProjectStateSnapshot{}, err
	}
	reportsByCluster := make(map[string]store.ClusterReport)
	hosts := make(map[string]struct{})
	for _, cluster := range clusters {
		hosts[cluster.HostID] = struct{}{}
	}
	for hostID := range hosts {
		reports, err := h.store.ListClusterReportsForHost(ctx, hostID, h.now().UTC())
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return ProjectStateSnapshot{}, err
		}
		for _, report := range reports {
			reportsByCluster[report.ClusterID] = report
		}
	}

	snapshot := ProjectStateSnapshot{
		Type: "project_state", ProjectID: projectID,
		Clusters: make([]ProjectClusterState, 0, len(clusters)),
	}
	for _, cluster := range clusters {
		state := ProjectClusterState{ClusterID: cluster.ID, HostID: cluster.HostID, Health: "unknown"}
		if report, ok := reportsByCluster[cluster.ID]; ok {
			state.ActualState = report.ActualState
			state.Health = report.Health
			state.LastSeen = &report.LastSeen
			state.Stale = report.Stale
		}
		snapshot.Clusters = append(snapshot.Clusters, state)
	}
	return snapshot, nil
}
