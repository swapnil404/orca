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
