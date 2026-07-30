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

-- name: GetHost :one
SELECT id, user_id, token_hash, token_expires_at, status, created_at, connected_at
FROM hosts
WHERE id = $1;

-- name: RotateHostToken :execrows
UPDATE hosts
SET token_hash = sqlc.arg(token_hash),
    token_expires_at = sqlc.arg(token_expires_at)
WHERE id = sqlc.arg(id)
  AND user_id = sqlc.arg(user_id)
  AND status = 'never_connected';

-- name: DeleteUnusedHost :execrows
WITH target_host AS (
    SELECT h.id
    FROM hosts h
    WHERE h.id = sqlc.arg(id)
      AND h.user_id = sqlc.arg(user_id)
      AND h.status = 'never_connected'
      AND NOT EXISTS (
          SELECT 1 FROM clusters c
          WHERE c.host_id = h.id AND c.deleted_at IS NULL
      )
), deleted_states AS (
    DELETE FROM desired_states ds
    USING clusters c, target_host h
    WHERE ds.cluster_id = c.id AND c.host_id = h.id
), deleted_clusters AS (
    DELETE FROM clusters c
    USING target_host h
    WHERE c.host_id = h.id
)
DELETE FROM hosts h
USING target_host target
WHERE h.id = target.id;
