package store

import (
	"context"
	"time"

	"github.com/swapnil404/orca/server/internal/store/sqlcdb"
)

// AlertComparison describes how an observed metric is compared with a threshold.
type AlertComparison string

const (
	// AlertComparisonGreaterThan fires when the metric is greater than the threshold.
	AlertComparisonGreaterThan AlertComparison = "gt"
	// AlertComparisonGreaterThanOrEqual fires when the metric is greater than or equal to the threshold.
	AlertComparisonGreaterThanOrEqual AlertComparison = "gte"
	// AlertComparisonLessThan fires when the metric is less than the threshold.
	AlertComparisonLessThan AlertComparison = "lt"
	// AlertComparisonLessThanOrEqual fires when the metric is less than or equal to the threshold.
	AlertComparisonLessThanOrEqual AlertComparison = "lte"
	// AlertComparisonEqual fires when the metric equals the threshold.
	AlertComparisonEqual AlertComparison = "eq"
	// AlertComparisonNotEqual fires when the metric does not equal the threshold.
	AlertComparisonNotEqual AlertComparison = "neq"
)

// AlertRuleState is the current evaluation state of an alert rule.
type AlertRuleState string

const (
	// AlertRuleStateOK means the rule is not currently firing.
	AlertRuleStateOK AlertRuleState = "ok"
	// AlertRuleStateFiring means the rule is currently firing.
	AlertRuleStateFiring AlertRuleState = "firing"
)

// AlertSeverity describes the operational impact assigned to an alert rule.
type AlertSeverity string

const (
	// AlertSeverityInfo identifies an informational alert.
	AlertSeverityInfo AlertSeverity = "info"
	// AlertSeverityWarning identifies an alert that needs attention.
	AlertSeverityWarning AlertSeverity = "warning"
	// AlertSeverityCritical identifies an alert that requires urgent action.
	AlertSeverityCritical AlertSeverity = "critical"
)

// AlertRule defines a threshold check scoped to a project or one cluster.
type AlertRule struct {
	ID                   string          `json:"id"`
	ProjectID            string          `json:"project_id"`
	ClusterID            string          `json:"cluster_id,omitempty"`
	MetricName           string          `json:"metric_name"`
	Comparison           AlertComparison `json:"comparison"`
	Threshold            float64         `json:"threshold"`
	DurationBeforeFiring time.Duration   `json:"-"`
	DurationSeconds      int64           `json:"duration_before_firing_seconds"`
	CurrentState         AlertRuleState  `json:"current_state"`
	LastTransitionAt     time.Time       `json:"last_transition_at"`
	Severity             AlertSeverity   `json:"severity"`
}

// AlertIncident is one persisted firing and optional resolution of a rule.
type AlertIncident struct {
	ID         int64           `json:"id"`
	ProjectID  string          `json:"project_id"`
	RuleID     string          `json:"rule_id"`
	MetricName string          `json:"metric_name"`
	Comparison AlertComparison `json:"comparison"`
	Threshold  float64         `json:"threshold"`
	Severity   AlertSeverity   `json:"severity"`
	FiredAt    time.Time       `json:"fired_at"`
	ResolvedAt *time.Time      `json:"resolved_at"`
}

// GlobalAlertIncident is an incident paired with its project for cross-project views.
type GlobalAlertIncident struct {
	AlertIncident
	ProjectName string `json:"project_name"`
}

// ListAlertRulesForProject returns rules for an active project owned by the user.
func (s *Postgres) ListAlertRulesForProject(ctx context.Context, userID, projectID string) ([]AlertRule, error) {
	rows, err := s.queries.ListAlertRulesForProject(ctx, sqlcdb.ListAlertRulesForProjectParams{
		ProjectID: projectID,
		UserID:    userID,
	})
	if err != nil {
		return nil, err
	}
	return alertRulesFromSQLC(rows), nil
}

// ListAlertIncidentsForProject returns newest-first firing history for a project.
func (s *Postgres) ListAlertIncidentsForProject(ctx context.Context, projectID string) ([]AlertIncident, error) {
	rows, err := s.queries.ListAlertIncidentsForProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	incidents := make([]AlertIncident, 0, len(rows))
	for _, row := range rows {
		incident := AlertIncident{
			ID: row.ID, ProjectID: row.ProjectID, RuleID: row.RuleID,
			MetricName: row.MetricName, Comparison: AlertComparison(row.Comparison),
			Threshold: row.Threshold, Severity: AlertSeverity(row.Severity), FiredAt: row.FiredAt,
		}
		if row.ResolvedAt.Valid {
			resolvedAt := row.ResolvedAt.Time
			incident.ResolvedAt = &resolvedAt
		}
		incidents = append(incidents, incident)
	}
	return incidents, nil
}

// ListAlertIncidentsForUser returns incidents from projects visible through the user's memberships.
func (s *Postgres) ListAlertIncidentsForUser(ctx context.Context, userID string) ([]GlobalAlertIncident, error) {
	rows, err := s.queries.ListAlertIncidentsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	incidents := make([]GlobalAlertIncident, 0, len(rows))
	for _, row := range rows {
		incident := GlobalAlertIncident{
			AlertIncident: AlertIncident{
				ID: row.ID, ProjectID: row.ProjectID, RuleID: row.RuleID,
				MetricName: row.MetricName, Comparison: AlertComparison(row.Comparison),
				Threshold: row.Threshold, Severity: AlertSeverity(row.Severity), FiredAt: row.FiredAt,
			},
			ProjectName: row.ProjectName,
		}
		if row.ResolvedAt.Valid {
			resolvedAt := row.ResolvedAt.Time
			incident.ResolvedAt = &resolvedAt
		}
		incidents = append(incidents, incident)
	}
	return incidents, nil
}

// ListAlertRulesForEvaluation returns rules whose project and cluster scopes are active.
func (s *Postgres) ListAlertRulesForEvaluation(ctx context.Context) ([]AlertRule, error) {
	rows, err := s.queries.ListAlertRulesForEvaluation(ctx)
	if err != nil {
		return nil, err
	}
	return alertRulesFromSQLC(rows), nil
}

// UpdateAlertRuleState records the current state and timestamp of a real transition.
func (s *Postgres) UpdateAlertRuleState(ctx context.Context, ruleID string, state AlertRuleState, transitionedAt time.Time) (AlertRule, error) {
	rule, err := s.queries.UpdateAlertRuleState(ctx, sqlcdb.UpdateAlertRuleStateParams{
		RuleID:         ruleID,
		CurrentState:   string(state),
		TransitionedAt: transitionedAt,
	})
	if err != nil {
		return AlertRule{}, err
	}
	return AlertRule{
		ID: rule.ID, ProjectID: rule.ProjectID, ClusterID: rule.ClusterID.String,
		MetricName: rule.MetricName, Comparison: AlertComparison(rule.Comparison), Threshold: rule.Threshold,
		DurationBeforeFiring: time.Duration(rule.DurationBeforeFiringSeconds) * time.Second,
		DurationSeconds:      rule.DurationBeforeFiringSeconds, CurrentState: AlertRuleState(rule.CurrentState),
		LastTransitionAt: rule.LastTransitionAt, Severity: AlertSeverity(rule.Severity),
	}, nil
}

func alertRulesFromSQLC(rows []sqlcdb.AlertRule) []AlertRule {
	rules := make([]AlertRule, 0, len(rows))
	for _, row := range rows {
		rules = append(rules, alertRuleFromSQLC(row))
	}
	return rules
}

func alertRuleFromSQLC(rule sqlcdb.AlertRule) AlertRule {
	return AlertRule{
		ID:                   rule.ID,
		ProjectID:            rule.ProjectID,
		ClusterID:            rule.ClusterID.String,
		MetricName:           rule.MetricName,
		Comparison:           AlertComparison(rule.Comparison),
		Threshold:            rule.Threshold,
		DurationBeforeFiring: time.Duration(rule.DurationBeforeFiringSeconds) * time.Second,
		DurationSeconds:      rule.DurationBeforeFiringSeconds,
		CurrentState:         AlertRuleState(rule.CurrentState),
		LastTransitionAt:     rule.LastTransitionAt,
		Severity:             AlertSeverity(rule.Severity),
	}
}
