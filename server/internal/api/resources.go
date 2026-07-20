package api

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/betterorca/betterorca/server/internal/store"
)

const maxRequestBodyBytes = 1 << 20

type userIDContextKey struct{}

// WithUserID associates an authenticated user ID with a request context.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDContextKey{}, userID)
}

type resourceStore interface {
	CreateProject(context.Context, store.CreateProjectParams) (store.Project, error)
	ListProjects(context.Context, string) ([]store.Project, error)
	GetProject(context.Context, string, string) (store.Project, error)
	UpdateProject(context.Context, store.UpdateProjectParams) (store.Project, error)
	DeleteProject(context.Context, string, string) error
	CreateCluster(context.Context, store.CreateClusterParams) (store.Cluster, error)
	ListClusters(context.Context, string, string) ([]store.Cluster, error)
	GetCluster(context.Context, string, string) (store.Cluster, error)
	UpdateCluster(context.Context, store.UpdateClusterParams) (store.Cluster, error)
	DeleteCluster(context.Context, string, string) error
}

type desiredStatePusher interface {
	PushDesiredState(context.Context, string) error
}

// ResourceHandler serves user-scoped project and cluster endpoints.
type ResourceHandler struct {
	store  resourceStore
	random io.Reader
	pusher desiredStatePusher
}

// NewResourceHandler creates the project and cluster API handler.
func NewResourceHandler(resources resourceStore, pushers ...desiredStatePusher) *ResourceHandler {
	handler := &ResourceHandler{store: resources, random: rand.Reader}
	if len(pushers) > 0 {
		handler.pusher = pushers[0]
	}
	return handler
}

// RegisterRoutes registers project and cluster routes on mux.
func (h *ResourceHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /projects", h.listProjects)
	mux.HandleFunc("POST /projects", h.createProject)
	mux.HandleFunc("GET /projects/{projectID}", h.getProject)
	mux.HandleFunc("PUT /projects/{projectID}", h.updateProject)
	mux.HandleFunc("DELETE /projects/{projectID}", h.deleteProject)
	mux.HandleFunc("GET /projects/{projectID}/clusters", h.listClusters)
	mux.HandleFunc("POST /projects/{projectID}/clusters", h.createCluster)
	mux.HandleFunc("GET /clusters/{clusterID}", h.getCluster)
	mux.HandleFunc("PUT /clusters/{clusterID}", h.updateCluster)
	mux.HandleFunc("DELETE /clusters/{clusterID}", h.deleteCluster)
}

func (h *ResourceHandler) createProject(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	var request struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	if strings.TrimSpace(request.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	id, err := randomID(h.random)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate project ID")
		return
	}
	project, err := h.store.CreateProject(r.Context(), store.CreateProjectParams{
		ID: id, UserID: userID, Name: strings.TrimSpace(request.Name),
	})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, project)
}

func (h *ResourceHandler) listProjects(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	projects, err := h.store.ListProjects(r.Context(), userID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projects)
}

func (h *ResourceHandler) getProject(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	project, err := h.store.GetProject(r.Context(), userID, r.PathValue("projectID"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, project)
}

func (h *ResourceHandler) updateProject(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	var request struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	request.Name = strings.TrimSpace(request.Name)
	if request.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	project, err := h.store.UpdateProject(r.Context(), store.UpdateProjectParams{
		ID: r.PathValue("projectID"), UserID: userID, Name: request.Name,
	})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, project)
}

func (h *ResourceHandler) deleteProject(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	projectID := r.PathValue("projectID")
	clusters, err := h.store.ListClusters(r.Context(), userID, projectID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if err := h.store.DeleteProject(r.Context(), userID, projectID); err != nil {
		writeStoreError(w, err)
		return
	}
	h.pushHosts(r.Context(), clusterHostIDs(clusters)...)
	w.WriteHeader(http.StatusNoContent)
}

type clusterRequest struct {
	HostID           string            `json:"host_id"`
	Name             string            `json:"name"`
	PostgresVersion  string            `json:"postgres_version"`
	Parameters       map[string]string `json:"parameters"`
	ReplicaCount     int32             `json:"replica_count"`
	PgBouncerEnabled bool              `json:"pgbouncer_enabled"`
}

func (h *ResourceHandler) createCluster(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	var request clusterRequest
	if !decodeJSON(w, r, &request) || !validateClusterRequest(w, request, true) {
		return
	}
	id, err := randomID(h.random)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate cluster ID")
		return
	}
	cluster, err := h.store.CreateCluster(r.Context(), store.CreateClusterParams{
		ID: id, UserID: userID, ProjectID: r.PathValue("projectID"), HostID: request.HostID,
		Name: strings.TrimSpace(request.Name), PostgresVersion: strings.TrimSpace(request.PostgresVersion),
		Parameters: normalizeParameters(request.Parameters), ReplicaCount: request.ReplicaCount,
		PgBouncerEnabled: request.PgBouncerEnabled,
	})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	h.pushHosts(r.Context(), cluster.HostID)
	writeJSON(w, http.StatusCreated, cluster)
}

func (h *ResourceHandler) listClusters(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	clusters, err := h.store.ListClusters(r.Context(), userID, r.PathValue("projectID"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, clusters)
}

func (h *ResourceHandler) getCluster(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	cluster, err := h.store.GetCluster(r.Context(), userID, r.PathValue("clusterID"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cluster)
}

func (h *ResourceHandler) updateCluster(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	var request clusterRequest
	if !decodeJSON(w, r, &request) || !validateClusterRequest(w, request, false) {
		return
	}
	if strings.TrimSpace(request.HostID) != "" {
		writeError(w, http.StatusBadRequest, "host_id cannot be changed")
		return
	}
	cluster, err := h.store.UpdateCluster(r.Context(), store.UpdateClusterParams{
		ID: r.PathValue("clusterID"), UserID: userID, Name: strings.TrimSpace(request.Name),
		PostgresVersion: strings.TrimSpace(request.PostgresVersion),
		Parameters:      normalizeParameters(request.Parameters), ReplicaCount: request.ReplicaCount,
		PgBouncerEnabled: request.PgBouncerEnabled,
	})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	h.pushHosts(r.Context(), cluster.HostID)
	writeJSON(w, http.StatusOK, cluster)
}

func (h *ResourceHandler) deleteCluster(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	clusterID := r.PathValue("clusterID")
	cluster, err := h.store.GetCluster(r.Context(), userID, clusterID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if err := h.store.DeleteCluster(r.Context(), userID, clusterID); err != nil {
		writeStoreError(w, err)
		return
	}
	h.pushHosts(r.Context(), cluster.HostID)
	w.WriteHeader(http.StatusNoContent)
}

func (h *ResourceHandler) pushHosts(ctx context.Context, hostIDs ...string) {
	if h.pusher == nil {
		return
	}
	for _, hostID := range hostIDs {
		// The desired state is already durable. Reconnection will recover a failed push.
		_ = h.pusher.PushDesiredState(ctx, hostID)
	}
}

func clusterHostIDs(clusters []store.Cluster) []string {
	hostIDs := make([]string, 0, len(clusters))
	seen := make(map[string]struct{}, len(clusters))
	for _, cluster := range clusters {
		if _, exists := seen[cluster.HostID]; exists {
			continue
		}
		seen[cluster.HostID] = struct{}{}
		hostIDs = append(hostIDs, cluster.HostID)
	}
	return hostIDs
}

func requireUserID(w http.ResponseWriter, r *http.Request) (string, bool) {
	userID, ok := r.Context().Value(userIDContextKey{}).(string)
	if !ok || strings.TrimSpace(userID) == "" {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return "", false
	}
	return userID, true
}

func validateClusterRequest(w http.ResponseWriter, request clusterRequest, requireHost bool) bool {
	if requireHost && strings.TrimSpace(request.HostID) == "" {
		writeError(w, http.StatusBadRequest, "host_id is required")
		return false
	}
	if strings.TrimSpace(request.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return false
	}
	if strings.TrimSpace(request.PostgresVersion) == "" {
		writeError(w, http.StatusBadRequest, "postgres_version is required")
		return false
	}
	if request.ReplicaCount < 0 {
		writeError(w, http.StatusBadRequest, "replica_count cannot be negative")
		return false
	}
	return true
}

func normalizeParameters(parameters map[string]string) map[string]string {
	if parameters == nil {
		return map[string]string{}
	}
	return parameters
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON request")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "request body must contain one JSON object")
		return false
	}
	return true
}

func writeStoreError(w http.ResponseWriter, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "resource not found")
		return
	}
	writeError(w, http.StatusInternalServerError, "internal server error")
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, struct {
		Error string `json:"error"`
	}{Error: message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
