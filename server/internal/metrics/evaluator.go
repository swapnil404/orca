package metrics

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/swapnil404/orca/server/internal/store"
)

const defaultEvaluationInterval = 15 * time.Second

type reportStore interface {
	ListMetricClusterReports(context.Context, string, time.Time) ([]store.MetricClusterReport, error)
}

type alertRuleStore interface {
	ListAlertRulesForEvaluation(context.Context) ([]store.AlertRule, error)
	UpdateAlertRuleState(context.Context, string, store.AlertRuleState, time.Time) (store.AlertRule, error)
}

// Evaluator periodically evaluates alert rules against current persisted metrics.
type Evaluator struct {
	reports  reportStore
	rules    alertRuleStore
	interval time.Duration
	now      func() time.Time
	logger   *slog.Logger

	mu    sync.Mutex
	state ruleStateMachine
}

// NewEvaluator creates an alert evaluator. Non-positive intervals use a 15-second default.
func NewEvaluator(reports reportStore, rules alertRuleStore, interval time.Duration) *Evaluator {
	if interval <= 0 {
		interval = defaultEvaluationInterval
	}
	return &Evaluator{
		reports: reports, rules: rules, interval: interval, now: time.Now,
		logger: slog.Default(), state: ruleStateMachine{pendingSince: make(map[string]time.Time)},
	}
}

// Run evaluates rules on each tick until ctx is canceled.
func (e *Evaluator) Run(ctx context.Context) {
	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := e.EvaluateOnce(ctx); err != nil {
				e.logger.Error("failed to evaluate alert rules", "error", err)
			}
		}
	}
}

// EvaluateOnce loads current reports and rules, then persists every state transition.
func (e *Evaluator) EvaluateOnce(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := e.now().UTC()
	rules, err := e.rules.ListAlertRulesForEvaluation(ctx)
	if err != nil {
		return fmt.Errorf("list alert rules: %w", err)
	}
	reports, err := e.reports.ListMetricClusterReports(ctx, "", now)
	if err != nil {
		return fmt.Errorf("list metric reports: %w", err)
	}
	values := currentMetricValues(reports, now)
	activeRuleIDs := make(map[string]struct{}, len(rules))
	var transitionErrors []error
	for _, rule := range rules {
		activeRuleIDs[rule.ID] = struct{}{}
		nextState, transitioned := e.state.evaluate(rule, values, now)
		if !transitioned {
			continue
		}
		if _, err := e.rules.UpdateAlertRuleState(ctx, rule.ID, nextState, now); err != nil {
			transitionErrors = append(transitionErrors, fmt.Errorf("update alert rule %q: %w", rule.ID, err))
		}
	}
	e.state.prune(activeRuleIDs)
	return errors.Join(transitionErrors...)
}

type ruleStateMachine struct {
	pendingSince map[string]time.Time
}

func (s *ruleStateMachine) evaluate(rule store.AlertRule, values []metricValue, now time.Time) (store.AlertRuleState, bool) {
	condition := evaluateRuleCondition(rule, values)
	if rule.CurrentState == store.AlertRuleStateFiring {
		delete(s.pendingSince, rule.ID)
		if condition != conditionFalse {
			return store.AlertRuleStateFiring, false
		}
		return store.AlertRuleStateOK, true
	}
	if condition != conditionTrue {
		delete(s.pendingSince, rule.ID)
		return store.AlertRuleStateOK, false
	}

	pendingSince, pending := s.pendingSince[rule.ID]
	if !pending {
		pendingSince = now
		s.pendingSince[rule.ID] = pendingSince
	}
	if now.Sub(pendingSince) >= rule.DurationBeforeFiring {
		return store.AlertRuleStateFiring, true
	}
	return store.AlertRuleStateOK, false
}

func (s *ruleStateMachine) prune(activeRuleIDs map[string]struct{}) {
	for ruleID := range s.pendingSince {
		if _, active := activeRuleIDs[ruleID]; !active {
			delete(s.pendingSince, ruleID)
		}
	}
}

type conditionResult uint8

const (
	conditionUnknown conditionResult = iota
	conditionFalse
	conditionTrue
)

func evaluateRuleCondition(rule store.AlertRule, values []metricValue) conditionResult {
	known := false
	unknown := false
	for _, metric := range values {
		if metric.name != rule.MetricName || metric.projectID != rule.ProjectID {
			continue
		}
		if rule.ClusterID != "" && metric.clusterID != rule.ClusterID {
			continue
		}
		if !metric.known {
			unknown = true
			continue
		}
		known = true
		if metric.sample && compareMetric(metric.value, rule.Comparison, rule.Threshold) {
			return conditionTrue
		}
	}
	if unknown || !known {
		return conditionUnknown
	}
	return conditionFalse
}

func compareMetric(value float64, comparison store.AlertComparison, threshold float64) bool {
	switch comparison {
	case store.AlertComparisonGreaterThan:
		return value > threshold
	case store.AlertComparisonGreaterThanOrEqual:
		return value >= threshold
	case store.AlertComparisonLessThan:
		return value < threshold
	case store.AlertComparisonLessThanOrEqual:
		return value <= threshold
	case store.AlertComparisonEqual:
		return value == threshold
	case store.AlertComparisonNotEqual:
		return value != threshold
	default:
		return false
	}
}
