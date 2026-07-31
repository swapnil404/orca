package api

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"regexp"
	"strings"
	"time"

	"github.com/swapnil404/orca/server/internal/auth"
	"github.com/swapnil404/orca/server/internal/store"
)

const (
	maxBackupIntervalSeconds = int64((1<<63 - 1) / 1_000_000_000)
	// Each replica is a container on the same registered host. Ten keeps a
	// request practical while allowing substantially larger test topologies.
	maxReplicaCount = int32(10)
)

const maxRequestBodyBytes = 1 << 20

// WithUserID associates an authenticated user ID with a request context.
func WithUserID(ctx context.Context, userID string) context.Context {
	return auth.WithUserID(ctx, userID)
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
	UpdatePgBouncer(context.Context, store.UpdatePgBouncerParams) (store.Cluster, error)
	UpdatePgHba(context.Context, store.UpdatePgHbaParams) (store.Cluster, error)
	UpdateParameters(context.Context, store.UpdateParametersParams) (store.Cluster, error)
	RestartProject(context.Context, store.RestartProjectParams) ([]store.Cluster, error)
	DeleteCluster(context.Context, string, string) error
	GetHost(context.Context, string) (store.Host, error)
}

type desiredStatePusher interface {
	PushDesiredState(context.Context, string) error
}

type hostConnectionLookup interface {
	IsConnected(string) bool
}

type projectChangeNotifier interface {
	NotifyProjectChange(context.Context, string) error
}

// ResourceHandler serves user-scoped project and cluster endpoints.
type ResourceHandler struct {
	store    resourceStore
	random   io.Reader
	pusher   desiredStatePusher
	hosts    hostConnectionLookup
	notifier projectChangeNotifier
}

// SetHostConnectionLookup supplies the live agent-session source used by host status responses.
func (h *ResourceHandler) SetHostConnectionLookup(hosts hostConnectionLookup) {
	h.hosts = hosts
}

// SetProjectChangeNotifier supplies the frontend project event publisher.
func (h *ResourceHandler) SetProjectChangeNotifier(notifier projectChangeNotifier) {
	h.notifier = notifier
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
	mux.HandleFunc("POST /orgs/{organizationID}/projects", h.createProject)
	mux.HandleFunc("GET /projects/{projectID}", h.getProject)
	mux.HandleFunc("PUT /projects/{projectID}", h.updateProject)
	mux.HandleFunc("DELETE /projects/{projectID}", h.deleteProject)
	mux.HandleFunc("POST /projects/{projectID}/restart", h.restartProject)
	mux.HandleFunc("GET /projects/{projectID}/clusters", h.listClusters)
	mux.HandleFunc("GET /projects/{projectID}/hosts", h.listProjectHosts)
	mux.HandleFunc("POST /projects/{projectID}/clusters", h.createCluster)
	mux.HandleFunc("GET /clusters/{clusterID}", h.getCluster)
	mux.HandleFunc("PUT /clusters/{clusterID}", h.updateCluster)
	mux.HandleFunc("PUT /clusters/{clusterID}/pgbouncer", h.updatePgBouncer)
	mux.HandleFunc("PUT /clusters/{clusterID}/pg-hba", h.updatePgHba)
	mux.HandleFunc("PUT /clusters/{clusterID}/parameters", h.updateParameters)
	mux.HandleFunc("DELETE /clusters/{clusterID}", h.deleteCluster)
}

func (h *ResourceHandler) createProject(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	var request struct {
		Name           string `json:"name"`
		OrganizationID string `json:"organization_id"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	if strings.TrimSpace(request.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	organizationID := r.PathValue("organizationID")
	if organizationID == "" {
		organizationID = request.OrganizationID
	}
	if !validUUID(organizationID) {
		writeError(w, http.StatusBadRequest, "valid organization_id is required")
		return
	}
	id, err := randomID(h.random)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate project ID")
		return
	}
	project, err := h.store.CreateProject(r.Context(), store.CreateProjectParams{
		ID: id, UserID: userID, OrganizationID: organizationID, Name: strings.TrimSpace(request.Name),
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

func (h *ResourceHandler) restartProject(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	projectID := r.PathValue("projectID")
	clusters, err := h.store.RestartProject(r.Context(), store.RestartProjectParams{ProjectID: projectID, UserID: userID})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	h.pushHosts(r.Context(), clusterHostIDs(clusters)...)
	h.notifyProject(r.Context(), projectID)
	w.WriteHeader(http.StatusNoContent)
}

type clusterRequest struct {
	HostID            string                  `json:"host_id"`
	Name              string                  `json:"name"`
	PostgresVersion   string                  `json:"postgres_version"`
	Parameters        map[string]string       `json:"parameters"`
	ReplicaCount      int32                   `json:"replica_count"`
	EnabledExtensions []string                `json:"enabled_extensions"`
	PgBouncerEnabled  bool                    `json:"pgbouncer_enabled"`
	PgBouncer         store.PgBouncerConfig   `json:"pg_bouncer"`
	PgBackRest        *store.PgBackRestConfig `json:"pg_back_rest"`
	PgHbaRules        *[]store.PgHbaRule      `json:"pg_hba_rules"`
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
	replicas, err := generateReplicas(h.random, request.ReplicaCount)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate replica ID")
		return
	}
	cluster, err := h.store.CreateCluster(r.Context(), store.CreateClusterParams{
		ID: id, UserID: userID, ProjectID: r.PathValue("projectID"), HostID: request.HostID,
		Name: strings.TrimSpace(request.Name), PostgresVersion: strings.TrimSpace(request.PostgresVersion),
		Parameters: normalizeParameters(request.Parameters), ReplicaCount: request.ReplicaCount,
		Replicas:          replicas,
		EnabledExtensions: request.EnabledExtensions,
		PgBouncerEnabled:  request.PgBouncerEnabled,
		PgBouncer:         request.PgBouncer,
		PgBackRest:        request.PgBackRest,
		PgHbaRules:        requestedPgHbaRules(request.PgHbaRules, nil),
	})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	h.pushHosts(r.Context(), cluster.HostID)
	h.notifyProject(r.Context(), cluster.ProjectID)
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
	current, err := h.store.GetCluster(r.Context(), userID, r.PathValue("clusterID"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	replicas, err := resizeReplicas(h.random, current.Replicas, request.ReplicaCount)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate replica ID")
		return
	}
	cluster, err := h.store.UpdateCluster(r.Context(), store.UpdateClusterParams{
		ID: r.PathValue("clusterID"), UserID: userID, Name: strings.TrimSpace(request.Name),
		PostgresVersion: strings.TrimSpace(request.PostgresVersion),
		Parameters:      normalizeParameters(request.Parameters), ReplicaCount: request.ReplicaCount,
		Replicas:          replicas,
		EnabledExtensions: request.EnabledExtensions,
		PgBouncerEnabled:  request.PgBouncerEnabled,
		PgBouncer:         request.PgBouncer,
		PgBackRest:        request.PgBackRest,
		PgHbaRules:        requestedPgHbaRules(request.PgHbaRules, current.PgHbaRules),
	})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	h.pushHosts(r.Context(), cluster.HostID)
	h.notifyProject(r.Context(), cluster.ProjectID)
	writeJSON(w, http.StatusOK, cluster)
}

func generateReplicas(random io.Reader, count int32) ([]store.Replica, error) {
	return resizeReplicas(random, nil, count)
}

func resizeReplicas(random io.Reader, current []store.Replica, count int32) ([]store.Replica, error) {
	if count <= int32(len(current)) {
		return append([]store.Replica(nil), current[:count]...), nil
	}
	replicas := append([]store.Replica(nil), current...)
	for int32(len(replicas)) < count {
		id, err := randomID(random)
		if err != nil {
			return nil, err
		}
		replicas = append(replicas, store.Replica{ID: id})
	}
	return replicas, nil
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
	h.notifyProject(r.Context(), cluster.ProjectID)
	w.WriteHeader(http.StatusNoContent)
}

func (h *ResourceHandler) updatePgBouncer(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	var request store.PgBouncerConfig
	if !decodeJSON(w, r, &request) {
		return
	}
	switch request.PoolMode {
	case "session", "transaction", "statement":
	default:
		writeError(w, http.StatusBadRequest, "pool_mode is invalid")
		return
	}
	if request.MaxConnections <= 0 {
		writeError(w, http.StatusBadRequest, "max_connections must be greater than zero")
		return
	}
	current, err := h.store.GetCluster(r.Context(), userID, r.PathValue("clusterID"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	cluster, err := h.store.UpdatePgBouncer(r.Context(), store.UpdatePgBouncerParams{
		ID: current.ID, UserID: userID, PoolMode: request.PoolMode,
		MaxConnections: request.MaxConnections,
	})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	h.pushHosts(r.Context(), cluster.HostID)
	h.notifyProject(r.Context(), cluster.ProjectID)
	writeJSON(w, http.StatusOK, cluster)
}

func (h *ResourceHandler) updatePgHba(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	var request struct {
		Rules []store.PgHbaRule `json:"rules"`
	}
	if !decodeJSON(w, r, &request) || !validatePgHbaRules(w, request.Rules) {
		return
	}
	current, err := h.store.GetCluster(r.Context(), userID, r.PathValue("clusterID"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	cluster, err := h.store.UpdatePgHba(r.Context(), store.UpdatePgHbaParams{ID: current.ID, UserID: userID, Rules: normalizePgHbaRules(request.Rules)})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	h.pushHosts(r.Context(), cluster.HostID)
	h.notifyProject(r.Context(), cluster.ProjectID)
	writeJSON(w, http.StatusOK, cluster)
}

func (h *ResourceHandler) updateParameters(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	var request struct {
		Parameters map[string]string `json:"parameters"`
	}
	if !decodeJSON(w, r, &request) || !validateParameters(w, request.Parameters) {
		return
	}
	current, err := h.store.GetCluster(r.Context(), userID, r.PathValue("clusterID"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	cluster, err := h.store.UpdateParameters(r.Context(), store.UpdateParametersParams{ID: current.ID, UserID: userID, Parameters: normalizeParameters(request.Parameters)})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	h.pushHosts(r.Context(), cluster.HostID)
	h.notifyProject(r.Context(), cluster.ProjectID)
	writeJSON(w, http.StatusOK, cluster)
}

func (h *ResourceHandler) listProjectHosts(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	clusters, err := h.store.ListClusters(r.Context(), userID, r.PathValue("projectID"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	hosts := make([]store.Host, 0)
	seen := make(map[string]struct{}, len(clusters))
	for _, cluster := range clusters {
		if _, exists := seen[cluster.HostID]; exists {
			continue
		}
		seen[cluster.HostID] = struct{}{}
		host, err := h.store.GetHost(r.Context(), cluster.HostID)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		hosts = append(hosts, host)
	}
	type hostResponse struct {
		ID          string           `json:"id"`
		Status      store.HostStatus `json:"status"`
		ConnectedAt *time.Time       `json:"connected_at,omitempty"`
	}
	response := make([]hostResponse, len(hosts))
	for i, host := range hosts {
		status := host.Status
		if h.hosts != nil {
			status = store.HostStatusOffline
			if h.hosts.IsConnected(host.ID) {
				status = store.HostStatusOnline
			} else if host.Status == store.HostStatusNeverConnected {
				status = store.HostStatusNeverConnected
			}
		}
		response[i] = hostResponse{ID: host.ID, Status: status}
		if host.ConnectedAt.Valid {
			response[i].ConnectedAt = &host.ConnectedAt.Time
		}
	}
	writeJSON(w, http.StatusOK, response)
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

func (h *ResourceHandler) notifyProject(ctx context.Context, projectID string) {
	if h.notifier != nil {
		_ = h.notifier.NotifyProjectChange(ctx, projectID)
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
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
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
	if !validateParameters(w, request.Parameters) {
		return false
	}
	if request.PgHbaRules != nil && !validatePgHbaRules(w, *request.PgHbaRules) {
		return false
	}
	if request.ReplicaCount < 0 {
		writeError(w, http.StatusBadRequest, "replica_count cannot be negative")
		return false
	}
	if request.ReplicaCount > maxReplicaCount {
		writeError(w, http.StatusBadRequest, "replica_count cannot exceed 10")
		return false
	}
	if request.PgBouncerEnabled {
		switch request.PgBouncer.PoolMode {
		case "", "session", "transaction", "statement":
		default:
			writeError(w, http.StatusBadRequest, "pg_bouncer.pool_mode is invalid")
			return false
		}
		if request.PgBouncer.MaxConnections < 0 {
			writeError(w, http.StatusBadRequest, "pg_bouncer.max_connections must be greater than zero")
			return false
		}
	}
	if request.PgBackRest != nil {
		if strings.TrimSpace(request.PgBackRest.RepoPath) == "" || strings.ContainsAny(request.PgBackRest.RepoPath, "\r\n") {
			writeError(w, http.StatusBadRequest, "pg_back_rest.repo_path must be nonempty and single-line")
			return false
		}
		if request.PgBackRest.RetentionFull <= 0 || request.PgBackRest.RetentionDiff <= 0 {
			writeError(w, http.StatusBadRequest, "pgBackRest retention counts must be greater than zero")
			return false
		}
		if request.PgBackRest.FullIntervalSeconds < 0 || request.PgBackRest.DiffIntervalSeconds < 0 || request.PgBackRest.IncrIntervalSeconds < 0 {
			writeError(w, http.StatusBadRequest, "pgBackRest backup intervals cannot be negative")
			return false
		}
		if request.PgBackRest.FullIntervalSeconds > maxBackupIntervalSeconds || request.PgBackRest.DiffIntervalSeconds > maxBackupIntervalSeconds || request.PgBackRest.IncrIntervalSeconds > maxBackupIntervalSeconds {
			writeError(w, http.StatusBadRequest, "pgBackRest backup interval is too large")
			return false
		}
	}
	return true
}

func requestedPgHbaRules(requested *[]store.PgHbaRule, current []store.PgHbaRule) []store.PgHbaRule {
	if requested != nil {
		return normalizePgHbaRules(*requested)
	}
	if current != nil {
		return current
	}
	return store.DefaultPgHbaRules()
}

func normalizePgHbaRules(rules []store.PgHbaRule) []store.PgHbaRule {
	normalized := make([]store.PgHbaRule, len(rules))
	for index, rule := range rules {
		normalized[index] = store.PgHbaRule{
			Type: strings.TrimSpace(rule.Type), Database: strings.TrimSpace(rule.Database),
			User: strings.TrimSpace(rule.User), Address: strings.TrimSpace(rule.Address), Method: strings.TrimSpace(rule.Method),
		}
	}
	return normalized
}

func validatePgHbaRules(w http.ResponseWriter, rules []store.PgHbaRule) bool {
	if len(rules) > 100 {
		writeError(w, http.StatusBadRequest, "pg_hba_rules cannot exceed 100 rules")
		return false
	}
	for _, rule := range rules {
		if rule.Type != "host" && rule.Type != "hostssl" && rule.Type != "local" {
			writeError(w, http.StatusBadRequest, "pg_hba_rules contains an invalid type")
			return false
		}
		if !validPgHbaList(rule.Database) || !validPgHbaList(rule.User) {
			writeError(w, http.StatusBadRequest, "pg_hba_rules contains an invalid database or user")
			return false
		}
		if rule.Type == "local" {
			if strings.TrimSpace(rule.Address) != "" {
				writeError(w, http.StatusBadRequest, "local pg_hba_rules must not include an address")
				return false
			}
		} else if !validPgHbaAddress(rule.Address) {
			writeError(w, http.StatusBadRequest, "pg_hba_rules contains an invalid IP address or CIDR")
			return false
		}
		switch rule.Method {
		case "trust", "md5", "scram-sha-256", "reject":
		default:
			writeError(w, http.StatusBadRequest, "pg_hba_rules contains an invalid authentication method")
			return false
		}
	}
	return true
}

func validPgHbaAddress(value string) bool {
	value = strings.TrimSpace(value)
	if _, err := netip.ParsePrefix(value); err == nil {
		return true
	}
	_, err := netip.ParseAddr(value)
	return err == nil
}

func validPgHbaList(value string) bool {
	if value == "" || len(value) > 256 || strings.ContainsAny(value, "\x00\r\n\t ") {
		return false
	}
	for _, item := range strings.Split(value, ",") {
		if item == "" {
			return false
		}
		for _, char := range item {
			if char != '_' && char != '-' && char != '.' && (char < '0' || char > '9') && (char < 'A' || char > 'Z') && (char < 'a' || char > 'z') {
				return false
			}
		}
	}
	return true
}

func normalizeParameters(parameters map[string]string) map[string]string {
	normalized := make(map[string]string, len(parameters))
	for name, value := range parameters {
		normalized[strings.ToLower(strings.TrimSpace(name))] = value
	}
	return normalized
}

var postgresParameterNamePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]*$`)

var orcaManagedPostgresParameters = map[string]struct{}{
	"archive_command": {}, "archive_mode": {}, "config_file": {}, "data_directory": {},
	"hba_file": {}, "hot_standby": {}, "ident_file": {}, "listen_addresses": {}, "port": {},
	"primary_conninfo": {}, "primary_slot_name": {}, "recovery_target": {}, "recovery_target_action": {},
	"recovery_target_inclusive": {}, "recovery_target_lsn": {}, "recovery_target_name": {},
	"recovery_target_time": {}, "recovery_target_timeline": {}, "recovery_target_xid": {},
	"shared_preload_libraries": {}, "unix_socket_directories": {}, "wal_level": {},
}

func validateParameters(w http.ResponseWriter, parameters map[string]string) bool {
	if len(parameters) > 100 {
		writeError(w, http.StatusBadRequest, "parameters cannot exceed 100 entries")
		return false
	}
	seen := make(map[string]struct{}, len(parameters))
	for name, value := range parameters {
		normalized := strings.ToLower(strings.TrimSpace(name))
		if !postgresParameterNamePattern.MatchString(normalized) {
			writeError(w, http.StatusBadRequest, "parameters contains an invalid PostgreSQL parameter name")
			return false
		}
		if _, duplicate := seen[normalized]; duplicate {
			writeError(w, http.StatusBadRequest, "parameters contains duplicate names")
			return false
		}
		seen[normalized] = struct{}{}
		if _, managed := orcaManagedPostgresParameters[normalized]; managed {
			writeError(w, http.StatusBadRequest, "parameters."+normalized+" is managed by Orca")
			return false
		}
		if len(value) > 4096 || strings.ContainsAny(value, "\x00\r\n") {
			writeError(w, http.StatusBadRequest, "parameter values must be single-line and at most 4096 bytes")
			return false
		}
	}
	return true
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
	if errors.Is(err, store.ErrRestoreOperationInProgress) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
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
