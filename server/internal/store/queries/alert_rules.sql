-- name: CreateAlertRule :one
INSERT INTO alert_rules (
    id, project_id, cluster_id, metric_name, comparison, threshold,
    duration_before_firing_seconds
)
SELECT sqlc.arg(rule_id)::text, p.id, c.id, sqlc.arg(metric_name)::text,
       sqlc.arg(comparison)::text, sqlc.arg(threshold)::double precision,
       sqlc.arg(duration_before_firing_seconds)::bigint
FROM projects p
LEFT JOIN clusters c
    ON c.id = NULLIF(sqlc.arg(cluster_id)::text, '')
    AND c.project_id = p.id
    AND c.deleted_at IS NULL
WHERE p.id = sqlc.arg(project_id)
  AND p.user_id = sqlc.arg(user_id)
  AND p.deleted_at IS NULL
  AND (sqlc.arg(cluster_id)::text = '' OR c.id IS NOT NULL)
RETURNING id, project_id, cluster_id, metric_name, comparison, threshold,
          duration_before_firing_seconds, current_state, last_transition_at;

-- name: ListAlertRulesForProject :many
SELECT ar.id, ar.project_id, ar.cluster_id, ar.metric_name, ar.comparison,
       ar.threshold, ar.duration_before_firing_seconds, ar.current_state,
       ar.last_transition_at
FROM alert_rules ar
JOIN projects p ON p.id = ar.project_id
LEFT JOIN clusters c ON c.id = ar.cluster_id
WHERE ar.project_id = $1
  AND p.user_id = $2
  AND p.deleted_at IS NULL
  AND (ar.cluster_id IS NULL OR c.deleted_at IS NULL)
ORDER BY ar.id;

-- name: ListAlertRulesForEvaluation :many
SELECT ar.id, ar.project_id, ar.cluster_id, ar.metric_name, ar.comparison,
       ar.threshold, ar.duration_before_firing_seconds, ar.current_state,
       ar.last_transition_at
FROM alert_rules ar
JOIN projects p ON p.id = ar.project_id
LEFT JOIN clusters c ON c.id = ar.cluster_id
WHERE p.deleted_at IS NULL
  AND (ar.cluster_id IS NULL OR c.deleted_at IS NULL)
ORDER BY ar.project_id, ar.id;

-- name: UpdateAlertRuleState :one
UPDATE alert_rules
SET current_state = sqlc.arg(current_state)::text,
    last_transition_at = CASE
        WHEN current_state <> sqlc.arg(current_state)::text THEN sqlc.arg(transitioned_at)::timestamptz
        ELSE last_transition_at
    END
WHERE id = sqlc.arg(rule_id)
RETURNING id, project_id, cluster_id, metric_name, comparison, threshold,
          duration_before_firing_seconds, current_state, last_transition_at;

-- name: DeleteAlertRule :execrows
DELETE FROM alert_rules ar
USING projects p
WHERE ar.id = sqlc.arg(rule_id)
  AND ar.project_id = p.id
  AND p.user_id = sqlc.arg(user_id)
  AND p.deleted_at IS NULL;
