package metrics

import (
	"time"

	"github.com/swapnil404/orca/server/internal/store"
)

const (
	// MetricClusterUp reports whether a cluster primary is ready to accept PostgreSQL connections.
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
	known     bool
	sample    bool
}

func currentMetricValues(reports []store.MetricClusterReport, now time.Time) []metricValue {
	values := make([]metricValue, 0, len(reports))
	for _, report := range reports {
		values = append(values, metricValue{name: MetricClusterUp, projectID: report.ProjectID, clusterID: report.ClusterID})
		clusterUpIndex := len(values) - 1
		replicationLagIndex := -1
		poolUtilizationIndex := -1
		backupLastSuccessIndex := -1
		backupAgeIndex := -1
		if report.ReplicaCount > 0 {
			values = append(values, metricValue{name: MetricReplicaReplicationLagBytes, projectID: report.ProjectID, clusterID: report.ClusterID})
			replicationLagIndex = len(values) - 1
		}
		if report.PgBouncerEnabled {
			values = append(values, metricValue{name: MetricPgBouncerPoolUtilizationRatio, projectID: report.ProjectID, clusterID: report.ClusterID})
			poolUtilizationIndex = len(values) - 1
		}
		if report.PgBackRestEnabled {
			values = append(values,
				metricValue{name: MetricBackupLastSuccessTimestampSeconds, projectID: report.ProjectID, clusterID: report.ClusterID},
				metricValue{name: MetricBackupAgeSeconds, projectID: report.ProjectID, clusterID: report.ClusterID},
			)
			backupLastSuccessIndex = len(values) - 2
			backupAgeIndex = len(values) - 1
		}
		if report.Stale || report.ActualState == nil {
			continue
		}
		if report.ActualState.PostgresReady != nil && report.Health != "unknown" {
			up := 0.0
			if report.ActualState.GetPostgresReady() && (report.Health == "healthy" || report.Health == "degraded") {
				up = 1
			}
			values[clusterUpIndex] = metricValue{
				name: MetricClusterUp, projectID: report.ProjectID, clusterID: report.ClusterID, value: up, known: true, sample: true,
			}
		}

		replicasObserved := len(report.ActualState.GetReplicas()) > 0
		for _, replica := range report.ActualState.GetReplicas() {
			if replica.ReplicationLagBytes == nil {
				replicasObserved = false
				continue
			}
			values = append(values, metricValue{
				name: MetricReplicaReplicationLagBytes, projectID: report.ProjectID,
				clusterID: report.ClusterID, replicaID: replica.GetId(), value: float64(replica.GetReplicationLagBytes()), known: true, sample: true,
			})
		}
		if replicasObserved && replicationLagIndex >= 0 {
			values[replicationLagIndex].known = true
		}

		pgBouncer := report.ActualState.GetPgBouncer()
		if poolUtilizationIndex >= 0 && pgBouncer != nil && pgBouncer.ActiveClientConnections != nil &&
			pgBouncer.MaxClientConnections != nil && pgBouncer.GetMaxClientConnections() > 0 {
			values[poolUtilizationIndex].known = true
			values = append(values, metricValue{
				name: MetricPgBouncerPoolUtilizationRatio, projectID: report.ProjectID, clusterID: report.ClusterID,
				value: float64(pgBouncer.GetActiveClientConnections()) / float64(pgBouncer.GetMaxClientConnections()), known: true, sample: true,
			})
		}

		backup := report.ActualState.GetBackup()
		if backupLastSuccessIndex < 0 || backup == nil || backup.LastSuccessUnixSeconds == nil {
			continue
		}
		lastSuccess := backup.GetLastSuccessUnixSeconds()
		values[backupLastSuccessIndex].known = true
		values[backupAgeIndex].known = true
		age := now.Sub(time.Unix(lastSuccess, 0)).Seconds()
		if age < 0 {
			age = 0
		}
		values = append(values,
			metricValue{
				name: MetricBackupLastSuccessTimestampSeconds, projectID: report.ProjectID,
				clusterID: report.ClusterID, value: float64(lastSuccess), known: true, sample: true,
			},
			metricValue{
				name: MetricBackupAgeSeconds, projectID: report.ProjectID, clusterID: report.ClusterID, value: age, known: true, sample: true,
			},
		)
	}
	return values
}
