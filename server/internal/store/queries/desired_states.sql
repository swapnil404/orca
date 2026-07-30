-- name: CreateDesiredState :one
INSERT INTO desired_states (host_id, cluster_id, operation, state)
VALUES ($1, $2, $3, $4)
RETURNING id, host_id, cluster_id, operation, state, created_at;

-- name: ListDesiredStateHistory :many
SELECT ds.id, ds.host_id, ds.cluster_id, ds.operation, ds.state, ds.created_at
FROM desired_states ds
JOIN clusters c ON c.id = ds.cluster_id
JOIN projects p ON p.id = c.project_id
JOIN organization_memberships om ON om.organization_id = p.organization_id
WHERE ds.cluster_id = $1 AND om.user_id = $2
ORDER BY ds.id;

-- name: ListCurrentDesiredStatesForHost :many
SELECT latest.id, latest.host_id, latest.cluster_id, latest.operation,
       latest.state, latest.created_at
FROM (
    SELECT DISTINCT ON (ds.cluster_id)
           ds.id, ds.host_id, ds.cluster_id, ds.operation, ds.state, ds.created_at
    FROM desired_states ds
    WHERE ds.host_id = $1
    ORDER BY ds.cluster_id, ds.id DESC
) latest
WHERE latest.operation = 'upsert'
ORDER BY latest.cluster_id;

-- name: GetDesiredStateRevisionForHost :one
SELECT COALESCE(MAX(id), 0)::BIGINT
FROM desired_states
WHERE host_id = $1;
