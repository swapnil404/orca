package metrics

import (
	"time"

	"github.com/swapnil404/orca/server/internal/store"
)

const (
	// MetricClusterUp reports whether a cluster primary is running.
	MetricClusterUp = "orca_cluster_up"
	// MetricReplicaReplicationLagBytes reports replica lag in bytes.
	MetricReplicaReplicationLagBytes = "orca_replica_replication_lag_bytes"
	// MetricPgBouncerPoolUtilizationRatio reports active PgBouncer clients as a fraction of capacity.
	MetricPgBouncerPoolUtilizationRatio = "orca_pgbouncer_pool_utilization_ratio"
	// MetricBackupLastSuccessTimestampSeconds reports the latest successful backup time.
	MetricBackupLastSuccessTimestampSeconds = "orca_backup_last_success_timestamp_seconds"
	// MetricBackupAgeSeconds reports the age of the latest successful backup.
	MetricBackupAgeSeconds = "orca_backup_age_seconds"
)

type metricValue struct {
	name      string
	projectID string
	clusterID string
	replicaID string
	value     float64
}

func currentMetricValues(reports []store.MetricClusterReport, now time.Time) []metricValue {
	values := make([]metricValue, 0, len(reports))
	for _, report := range reports {
		up := 0.0
		if !report.Stale && report.ActualState != nil && report.ActualState.GetStatus() == "running" {
			up = 1
		}
		values = append(values, metricValue{
			name: MetricClusterUp, projectID: report.ProjectID, clusterID: report.ClusterID, value: up,
		})
		if report.Stale || report.ActualState == nil {
			continue
		}

		for _, replica := range report.ActualState.GetReplicas() {
			if replica.ReplicationLagBytes == nil {
				continue
			}
			values = append(values, metricValue{
				name: MetricReplicaReplicationLagBytes, projectID: report.ProjectID,
				clusterID: report.ClusterID, replicaID: replica.GetId(), value: float64(replica.GetReplicationLagBytes()),
			})
		}

		pgBouncer := report.ActualState.GetPgBouncer()
		if pgBouncer != nil && pgBouncer.ActiveClientConnections != nil &&
			pgBouncer.MaxClientConnections != nil && pgBouncer.GetMaxClientConnections() > 0 {
			values = append(values, metricValue{
				name: MetricPgBouncerPoolUtilizationRatio, projectID: report.ProjectID, clusterID: report.ClusterID,
				value: float64(pgBouncer.GetActiveClientConnections()) / float64(pgBouncer.GetMaxClientConnections()),
			})
		}

		backup := report.ActualState.GetBackup()
		if backup == nil || backup.LastSuccessUnixSeconds == nil {
			continue
		}
		lastSuccess := backup.GetLastSuccessUnixSeconds()
		age := now.Sub(time.Unix(lastSuccess, 0)).Seconds()
		if age < 0 {
			age = 0
		}
		values = append(values,
			metricValue{
				name: MetricBackupLastSuccessTimestampSeconds, projectID: report.ProjectID,
				clusterID: report.ClusterID, value: float64(lastSuccess),
			},
			metricValue{
				name: MetricBackupAgeSeconds, projectID: report.ProjectID, clusterID: report.ClusterID, value: age,
			},
		)
	}
	return values
}
