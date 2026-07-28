-- name: CreatePasswordUser :one
INSERT INTO users (id, email, password_hash)
VALUES ($1, LOWER($2), $3)
RETURNING id, email, password_hash, created_at, updated_at, deleted_at;

-- name: GetUserByEmail :one
SELECT id, email, password_hash, created_at, updated_at, deleted_at
FROM users
WHERE LOWER(email) = LOWER($1)
  AND deleted_at IS NULL;

-- name: UserIsActive :one
SELECT EXISTS (
    SELECT 1
    FROM users
    WHERE id = $1 AND deleted_at IS NULL
) AS active;

-- name: SoftDeleteUser :one
UPDATE users
SET deleted_at = NOW(), updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING id;

-- name: GetOAuthIdentityUserID :one
SELECT user_id
FROM oauth_identities
WHERE provider = $1 AND provider_user_id = $2;

-- name: CreateOAuthUser :one
INSERT INTO users (id)
VALUES ($1)
RETURNING id;

-- name: CreateOAuthIdentity :one
INSERT INTO oauth_identities (provider, provider_user_id, provider_email, user_id)
VALUES (sqlc.arg(provider), sqlc.arg(provider_user_id), NULLIF(sqlc.arg(provider_email)::text, ''), sqlc.arg(user_id))
RETURNING provider, provider_user_id, provider_email, user_id, created_at;
