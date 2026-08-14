-- name: ListAlertRulesForProject :many
SELECT ar.id, ar.project_id, ar.cluster_id, ar.metric_name, ar.comparison,
       ar.threshold, ar.duration_before_firing_seconds, ar.current_state,
       ar.last_transition_at, ar.severity
FROM alert_rules ar
JOIN projects p ON p.id = ar.project_id
JOIN organization_memberships om ON om.organization_id = p.organization_id
LEFT JOIN clusters c ON c.id = ar.cluster_id
WHERE ar.project_id = $1
  AND om.user_id = $2
  AND p.deleted_at IS NULL
  AND (ar.cluster_id IS NULL OR c.deleted_at IS NULL)
ORDER BY ar.id;

-- name: ListAlertRulesForEvaluation :many
SELECT ar.id, ar.project_id, ar.cluster_id, ar.metric_name, ar.comparison,
       ar.threshold, ar.duration_before_firing_seconds, ar.current_state,
       ar.last_transition_at, ar.severity
FROM alert_rules ar
JOIN projects p ON p.id = ar.project_id
LEFT JOIN clusters c ON c.id = ar.cluster_id
WHERE p.deleted_at IS NULL
  AND (ar.cluster_id IS NULL OR c.deleted_at IS NULL)
ORDER BY ar.project_id, ar.id;

-- name: UpdateAlertRuleState :one
WITH updated_rule AS (
    UPDATE alert_rules ar
    SET current_state = sqlc.arg(current_state)::text,
        last_transition_at = CASE
            WHEN ar.current_state <> sqlc.arg(current_state)::text THEN sqlc.arg(transitioned_at)::timestamptz
            ELSE ar.last_transition_at
        END
    WHERE ar.id = sqlc.arg(rule_id)
    RETURNING ar.id, ar.project_id, ar.cluster_id, ar.metric_name, ar.comparison, ar.threshold,
              ar.duration_before_firing_seconds, ar.current_state, ar.last_transition_at,
              ar.severity
), opened_incident AS (
    INSERT INTO alert_incidents (
        project_id, rule_id, metric_name, comparison, threshold, severity, fired_at
    )
    SELECT project_id, id, metric_name, comparison, threshold, severity,
           sqlc.arg(transitioned_at)::timestamptz
    FROM updated_rule
    WHERE current_state = 'firing'
    ON CONFLICT (rule_id) WHERE resolved_at IS NULL DO NOTHING
), resolved_incident AS (
    UPDATE alert_incidents
    SET resolved_at = sqlc.arg(transitioned_at)::timestamptz
    WHERE rule_id = sqlc.arg(rule_id)
      AND resolved_at IS NULL
      AND sqlc.arg(current_state)::text = 'ok'
)
SELECT id, project_id, cluster_id, metric_name, comparison, threshold,
       duration_before_firing_seconds, current_state, last_transition_at,
       severity
FROM updated_rule;

-- name: ListAlertIncidentsForProject :many
SELECT id, project_id, rule_id, metric_name, comparison, threshold, severity,
       fired_at, resolved_at
FROM alert_incidents
WHERE project_id = $1
ORDER BY fired_at DESC, id DESC;

-- name: ListAlertIncidentsForUser :many
SELECT ai.id, ai.project_id, p.name AS project_name, ai.rule_id,
       ai.metric_name, ai.comparison, ai.threshold, ai.severity,
       ai.fired_at, ai.resolved_at
FROM alert_incidents ai
JOIN projects p ON p.id = ai.project_id
JOIN organization_memberships om ON om.organization_id = p.organization_id
WHERE om.user_id = $1
  AND p.deleted_at IS NULL
ORDER BY ai.fired_at DESC, ai.id DESC;
