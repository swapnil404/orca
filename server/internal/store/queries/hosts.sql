-- name: CreateHost :one
INSERT INTO hosts (id, user_id, token_hash, token_expires_at, status)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, user_id, token_hash, token_expires_at, status, created_at, connected_at;

-- name: GetHostByTokenHash :one
SELECT id, user_id, token_hash, token_expires_at, status, created_at, connected_at
FROM hosts
WHERE token_hash = $1;

-- name: UpdateHostStatus :exec
UPDATE hosts
SET status = $2,
    connected_at = CASE WHEN $2 = 'online' THEN NOW() ELSE connected_at END
WHERE id = $1;

-- name: ListProjectHosts :many
SELECT DISTINCT h.id, h.user_id, h.token_hash, h.token_expires_at, h.status, h.created_at, h.connected_at
FROM hosts h
JOIN clusters c ON c.host_id = h.id AND c.deleted_at IS NULL
JOIN projects p ON p.id = c.project_id AND p.deleted_at IS NULL
JOIN organization_memberships om ON om.organization_id = p.organization_id
WHERE p.id = $1 AND om.user_id = $2
ORDER BY h.id;
