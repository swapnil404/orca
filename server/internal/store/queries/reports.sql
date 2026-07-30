-- name: UpsertAgentReport :exec
INSERT INTO agent_reports (host_id, actual_state, health_report, reconciliation_results, desired_state_revision, reported_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (host_id) DO UPDATE
SET actual_state = EXCLUDED.actual_state,
    health_report = EXCLUDED.health_report,
    reconciliation_results = EXCLUDED.reconciliation_results,
    desired_state_revision = EXCLUDED.desired_state_revision,
    reported_at = EXCLUDED.reported_at;

-- name: GetAgentReport :one
SELECT host_id, actual_state, health_report, reconciliation_results, desired_state_revision, reported_at
FROM agent_reports
WHERE host_id = $1;

-- name: DeleteClusterReportsForHost :exec
DELETE FROM cluster_reports
WHERE host_id = $1;

-- name: UpsertClusterReport :execrows
INSERT INTO cluster_reports (host_id, cluster_id, actual_state, health_status, desired_state_revision, reconciliation_results, reported_at)
SELECT $1, $2, $3, $4, $5, $6, $7
FROM clusters
WHERE id = $2 AND host_id = $1
ON CONFLICT (host_id, cluster_id) DO UPDATE
SET actual_state = EXCLUDED.actual_state,
    health_status = EXCLUDED.health_status,
    desired_state_revision = EXCLUDED.desired_state_revision,
    reconciliation_results = EXCLUDED.reconciliation_results,
    reported_at = EXCLUDED.reported_at;

-- name: ListClusterReportsForHost :many
SELECT host_id, cluster_id, actual_state, health_status, desired_state_revision, reconciliation_results, reported_at
FROM cluster_reports
WHERE host_id = $1
ORDER BY cluster_id;

-- name: ListMetricClusterReports :many
SELECT p.id AS project_id, c.id AS cluster_id,
       COALESCE(cr.actual_state, 'null'::jsonb) AS actual_state,
       COALESCE(cr.health_status, 'unknown') AS health_status,
       COALESCE(cr.reported_at, 'epoch'::timestamptz) AS reported_at
FROM projects p
JOIN clusters c ON c.project_id = p.id
LEFT JOIN cluster_reports cr ON cr.host_id = c.host_id AND cr.cluster_id = c.id
WHERE p.deleted_at IS NULL AND c.deleted_at IS NULL
  AND (sqlc.arg(project_id)::text = '' OR p.id = sqlc.arg(project_id))
ORDER BY p.id, c.id;

-- name: ListBackupJobs :many
SELECT p.id AS project_id, p.name AS project_name,
       c.id AS cluster_id, c.name AS cluster_name, c.pgbackrest_enabled,
       COALESCE(cr.actual_state, 'null'::jsonb) AS actual_state,
       COALESCE(cr.reported_at, 'epoch'::timestamptz) AS reported_at
FROM projects p
JOIN organization_memberships om ON om.organization_id = p.organization_id
JOIN clusters c ON c.project_id = p.id
LEFT JOIN cluster_reports cr ON cr.host_id = c.host_id AND cr.cluster_id = c.id
WHERE om.user_id = sqlc.arg(user_id)
  AND p.deleted_at IS NULL AND c.deleted_at IS NULL
  AND (sqlc.arg(project_id)::text = '' OR p.id = sqlc.arg(project_id))
ORDER BY p.name, p.id, c.name, c.id;
