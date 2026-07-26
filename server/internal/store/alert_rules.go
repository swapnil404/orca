package store

import (
	"context"
	"database/sql"
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

// AlertRule defines a threshold check scoped to a project or one cluster.
type AlertRule struct {
	ID                   string
	ProjectID            string
	ClusterID            string
	MetricName           string
	Comparison           AlertComparison
	Threshold            float64
	DurationBeforeFiring time.Duration
	CurrentState         AlertRuleState
	LastTransitionAt     time.Time
}

// CreateAlertRuleParams contains the values needed to create an alert rule.
type CreateAlertRuleParams struct {
	ID                   string
	UserID               string
	ProjectID            string
	ClusterID            string
	MetricName           string
	Comparison           AlertComparison
	Threshold            float64
	DurationBeforeFiring time.Duration
}

// CreateAlertRule persists an alert rule in an active project owned by the user.
func (s *Postgres) CreateAlertRule(ctx context.Context, params CreateAlertRuleParams) (AlertRule, error) {
	rule, err := s.queries.CreateAlertRule(ctx, sqlcdb.CreateAlertRuleParams{
		RuleID:                      params.ID,
		UserID:                      params.UserID,
		ProjectID:                   params.ProjectID,
		ClusterID:                   params.ClusterID,
		MetricName:                  params.MetricName,
		Comparison:                  string(params.Comparison),
		Threshold:                   params.Threshold,
		DurationBeforeFiringSeconds: int64(params.DurationBeforeFiring / time.Second),
	})
	if err != nil {
		return AlertRule{}, err
	}
	return alertRuleFromSQLC(rule), nil
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
	return alertRuleFromSQLC(rule), nil
}

// DeleteAlertRule deletes an alert rule owned by the user.
func (s *Postgres) DeleteAlertRule(ctx context.Context, userID, ruleID string) error {
	rows, err := s.queries.DeleteAlertRule(ctx, sqlcdb.DeleteAlertRuleParams{RuleID: ruleID, UserID: userID})
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
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
		CurrentState:         AlertRuleState(rule.CurrentState),
		LastTransitionAt:     rule.LastTransitionAt,
	}
}
