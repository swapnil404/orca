package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/swapnil404/orca/pkg/types"
	"github.com/swapnil404/orca/server/internal/store/sqlcdb"
)

const (
	// DefaultReportStalenessWindow is the maximum age of a report considered current.
	DefaultReportStalenessWindow = 2 * time.Minute
	unknownHealthStatus          = "unknown"
	currentReportStatus          = "current"
)

// HostReport is the latest actual state and health reported by one host.
type HostReport struct {
	HostID       string
	ActualState  *types.ActualState
	HealthReport *types.HealthReport
	LastSeen     time.Time
	Status       string
	Stale        bool
}

// ClusterReport is the latest actual state and health reported for one cluster.
type ClusterReport struct {
	HostID      string
	ClusterID   string
	ActualState *types.ActualCluster
	Health      string
	LastSeen    time.Time
	Stale       bool
}

// StoreAgentReport atomically replaces the latest report snapshot for hostID.
func (s *Postgres) StoreAgentReport(ctx context.Context, hostID string, report *types.AgentReportMessage, receivedAt time.Time) error {
	actualState, err := protojson.Marshal(report.GetActualState())
	if err != nil {
		return fmt.Errorf("marshal actual state: %w", err)
	}
	healthReport, err := protojson.Marshal(report.GetHealthReport())
	if err != nil {
		return fmt.Errorf("marshal health report: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	queries := s.queries.WithTx(tx)
	if err := queries.UpsertAgentReport(ctx, sqlcdb.UpsertAgentReportParams{
		HostID: hostID, ActualState: actualState, HealthReport: healthReport, ReportedAt: receivedAt,
	}); err != nil {
		return fmt.Errorf("store host report: %w", err)
	}
	if err := queries.DeleteClusterReportsForHost(ctx, hostID); err != nil {
		return fmt.Errorf("replace cluster reports: %w", err)
	}

	actualByID := make(map[string]*types.ActualCluster, len(report.GetActualState().GetClusters()))
	for _, cluster := range report.GetActualState().GetClusters() {
		actualByID[cluster.GetId()] = cluster
	}
	healthByID := make(map[string]types.ClusterStatus, len(report.GetHealthReport().GetClusters()))
	for _, health := range report.GetHealthReport().GetClusters() {
		healthByID[health.GetClusterId()] = health.GetStatus()
	}
	clusterIDs := make(map[string]struct{}, len(actualByID)+len(healthByID))
	for clusterID := range actualByID {
		clusterIDs[clusterID] = struct{}{}
	}
	for clusterID := range healthByID {
		clusterIDs[clusterID] = struct{}{}
	}
	for clusterID := range clusterIDs {
		actualJSON := json.RawMessage("null")
		if actual := actualByID[clusterID]; actual != nil {
			payload, marshalErr := protojson.Marshal(actual)
			err = marshalErr
			if err != nil {
				return fmt.Errorf("marshal actual cluster %q: %w", clusterID, err)
			}
			actualJSON = payload
		}
		health := unknownHealthStatus
		if status, ok := healthByID[clusterID]; ok {
			health = clusterHealthStatus(status)
		}
		rows, err := queries.UpsertClusterReport(ctx, sqlcdb.UpsertClusterReportParams{
			HostID: hostID, ClusterID: clusterID, ActualState: actualJSON,
			HealthStatus: health, ReportedAt: receivedAt,
		})
		if err != nil {
			return fmt.Errorf("store cluster report %q: %w", clusterID, err)
		}
		if rows == 0 {
			return fmt.Errorf("cluster %q does not belong to host %q", clusterID, hostID)
		}
	}
	return tx.Commit()
}

// GetHostReport returns the latest host report and marks it stale at read time.
func (s *Postgres) GetHostReport(ctx context.Context, hostID string, now time.Time) (HostReport, error) {
	row, err := s.queries.GetAgentReport(ctx, hostID)
	if err != nil {
		return HostReport{}, err
	}
	report := HostReport{
		HostID: hostID, ActualState: &types.ActualState{}, HealthReport: &types.HealthReport{},
		LastSeen: row.ReportedAt, Status: currentReportStatus,
	}
	if err := protojson.Unmarshal(row.ActualState, report.ActualState); err != nil {
		return HostReport{}, fmt.Errorf("decode actual state: %w", err)
	}
	if err := protojson.Unmarshal(row.HealthReport, report.HealthReport); err != nil {
		return HostReport{}, fmt.Errorf("decode health report: %w", err)
	}
	report.Stale = reportIsStale(report.LastSeen, now, DefaultReportStalenessWindow)
	if report.Stale {
		report.Status = unknownHealthStatus
	}
	return report, nil
}

// ListClusterReportsForHost returns the latest cluster reports, with stale health set to unknown.
func (s *Postgres) ListClusterReportsForHost(ctx context.Context, hostID string, now time.Time) ([]ClusterReport, error) {
	rows, err := s.queries.ListClusterReportsForHost(ctx, hostID)
	if err != nil {
		return nil, err
	}
	reports := make([]ClusterReport, 0, len(rows))
	for _, row := range rows {
		report := ClusterReport{
			HostID: row.HostID, ClusterID: row.ClusterID, Health: row.HealthStatus, LastSeen: row.ReportedAt,
		}
		if string(row.ActualState) != "null" {
			report.ActualState = &types.ActualCluster{}
			if err := protojson.Unmarshal(row.ActualState, report.ActualState); err != nil {
				return nil, fmt.Errorf("decode actual cluster %q: %w", row.ClusterID, err)
			}
		}
		report.Stale = reportIsStale(report.LastSeen, now, DefaultReportStalenessWindow)
		if report.Stale {
			report.Health = unknownHealthStatus
		}
		reports = append(reports, report)
	}
	return reports, nil
}

func reportIsStale(lastSeen, now time.Time, window time.Duration) bool {
	return now.Sub(lastSeen) > window
}

func clusterHealthStatus(status types.ClusterStatus) string {
	switch status {
	case types.ClusterStatus_CLUSTER_STATUS_PENDING:
		return "pending"
	case types.ClusterStatus_CLUSTER_STATUS_HEALTHY:
		return "healthy"
	case types.ClusterStatus_CLUSTER_STATUS_DEGRADED:
		return "degraded"
	case types.ClusterStatus_CLUSTER_STATUS_DOWN:
		return "down"
	default:
		return unknownHealthStatus
	}
}
