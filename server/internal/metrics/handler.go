// Package metrics exposes persisted agent observations in Prometheus format.
package metrics

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/swapnil404/orca/server/internal/auth"
	"github.com/swapnil404/orca/server/internal/store"
)

type reportStore interface {
	ListMetricClusterReports(context.Context, string, time.Time) ([]store.MetricClusterReport, error)
	GetProject(context.Context, string, string) (store.Project, error)
	ListProjects(context.Context, string) ([]store.Project, error)
}

// Handler serves global and project-filtered Prometheus metrics.
type Handler struct {
	store reportStore
	now   func() time.Time
}

// NewHandler creates a metrics handler backed by persisted agent reports.
func NewHandler(reports reportStore) *Handler {
	return &Handler{store: reports, now: time.Now}
}

// RegisterRoutes registers global and project-filtered metrics routes.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /metrics", h.serve)
	mux.HandleFunc("GET /projects/{projectID}/metrics", h.serve)
}

func (h *Handler) serve(w http.ResponseWriter, r *http.Request) {
	now := h.now().UTC()
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	reports, err := h.ownedReports(r.Context(), userID, r.PathValue("projectID"), now)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "resource not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to load metrics", http.StatusInternalServerError)
		return
	}

	registry := prometheus.NewRegistry()
	clusterUp := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "orca",
		Subsystem: "cluster",
		Name:      "up",
		Help:      "Whether the cluster primary is currently reported running.",
	}, []string{"project_id", "cluster_id"})
	replicationLag := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "orca",
		Subsystem: "replica",
		Name:      "replication_lag_bytes",
		Help:      "Latest reported replication lag for a replica in bytes.",
	}, []string{"project_id", "cluster_id", "replica_id"})
	poolUtilization := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "orca",
		Subsystem: "pgbouncer",
		Name:      "pool_utilization_ratio",
		Help:      "Ratio of active PgBouncer client connections to the configured maximum.",
	}, []string{"project_id", "cluster_id"})
	backupLastSuccess := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "orca",
		Subsystem: "backup",
		Name:      "last_success_timestamp_seconds",
		Help:      "Unix timestamp of the latest reported successful backup.",
	}, []string{"project_id", "cluster_id"})
	backupAge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "orca",
		Subsystem: "backup",
		Name:      "age_seconds",
		Help:      "Age of the latest reported successful backup in seconds.",
	}, []string{"project_id", "cluster_id"})
	registry.MustRegister(clusterUp, replicationLag, poolUtilization, backupLastSuccess, backupAge)

	for _, metric := range currentMetricValues(reports, now) {
		if !metric.known || !metric.sample {
			continue
		}
		labels := prometheus.Labels{"project_id": metric.projectID, "cluster_id": metric.clusterID}
		switch metric.name {
		case MetricClusterUp:
			clusterUp.With(labels).Set(metric.value)
		case MetricReplicaReplicationLagBytes:
			labels["replica_id"] = metric.replicaID
			replicationLag.With(labels).Set(metric.value)
		case MetricPgBouncerPoolUtilizationRatio:
			poolUtilization.With(labels).Set(metric.value)
		case MetricBackupLastSuccessTimestampSeconds:
			backupLastSuccess.With(labels).Set(metric.value)
		case MetricBackupAgeSeconds:
			backupAge.With(labels).Set(metric.value)
		}
	}

	promhttp.HandlerFor(registry, promhttp.HandlerOpts{}).ServeHTTP(w, r)
}

func (h *Handler) ownedReports(ctx context.Context, userID, projectID string, now time.Time) ([]store.MetricClusterReport, error) {
	if projectID != "" {
		if _, err := h.store.GetProject(ctx, userID, projectID); err != nil {
			return nil, err
		}
		return h.store.ListMetricClusterReports(ctx, projectID, now)
	}

	projects, err := h.store.ListProjects(ctx, userID)
	if err != nil {
		return nil, err
	}
	reports := make([]store.MetricClusterReport, 0)
	for _, project := range projects {
		projectReports, err := h.store.ListMetricClusterReports(ctx, project.ID, now)
		if err != nil {
			return nil, err
		}
		reports = append(reports, projectReports...)
	}
	return reports, nil
}
