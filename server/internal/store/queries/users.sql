-- name: CreatePasswordUser :one
INSERT INTO users (id, email, password_hash)
VALUES ($1, LOWER($2), $3)
RETURNING id, email, password_hash, created_at, updated_at;

-- name: GetUserByEmail :one
SELECT id, email, password_hash, created_at, updated_at
FROM users
WHERE LOWER(email) = LOWER($1);
