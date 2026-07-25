// Package metrics exposes persisted agent observations in Prometheus format.
package metrics

import (
	"context"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/swapnil404/orca/server/internal/store"
)

type reportStore interface {
	ListMetricClusterReports(context.Context, string, time.Time) ([]store.MetricClusterReport, error)
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
	reports, err := h.store.ListMetricClusterReports(r.Context(), r.PathValue("projectID"), now)
	if err != nil {
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

	for _, report := range reports {
		labels := prometheus.Labels{"project_id": report.ProjectID, "cluster_id": report.ClusterID}
		up := 0.0
		if !report.Stale && report.ActualState != nil && report.ActualState.GetStatus() == "running" {
			up = 1
		}
		clusterUp.With(labels).Set(up)
		if report.Stale || report.ActualState == nil {
			continue
		}

		for _, replica := range report.ActualState.GetReplicas() {
			if replica.ReplicationLagBytes == nil {
				continue
			}
			replicationLag.With(prometheus.Labels{
				"project_id": report.ProjectID,
				"cluster_id": report.ClusterID,
				"replica_id": replica.GetId(),
			}).Set(float64(replica.GetReplicationLagBytes()))
		}

		pgBouncer := report.ActualState.GetPgBouncer()
		if pgBouncer != nil && pgBouncer.ActiveClientConnections != nil &&
			pgBouncer.MaxClientConnections != nil && pgBouncer.GetMaxClientConnections() > 0 {
			poolUtilization.With(labels).Set(
				float64(pgBouncer.GetActiveClientConnections()) / float64(pgBouncer.GetMaxClientConnections()),
			)
		}

		backup := report.ActualState.GetBackup()
		if backup != nil && backup.LastSuccessUnixSeconds != nil {
			lastSuccess := backup.GetLastSuccessUnixSeconds()
			backupLastSuccess.With(labels).Set(float64(lastSuccess))
			age := now.Sub(time.Unix(lastSuccess, 0)).Seconds()
			if age < 0 {
				age = 0
			}
			backupAge.With(labels).Set(age)
		}
	}

	promhttp.HandlerFor(registry, promhttp.HandlerOpts{}).ServeHTTP(w, r)
}
