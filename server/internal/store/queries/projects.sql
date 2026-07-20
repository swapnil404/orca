-- name: CreateProject :one
INSERT INTO projects (id, user_id, name)
VALUES ($1, $2, $3)
RETURNING id, user_id, name, created_at, updated_at, deleted_at;

-- name: ListProjects :many
SELECT id, user_id, name, created_at, updated_at, deleted_at
FROM projects
WHERE user_id = $1 AND deleted_at IS NULL
ORDER BY created_at, id;

-- name: ListProjectIDsForHost :many
SELECT DISTINCT p.id
FROM projects p
JOIN clusters c ON c.project_id = p.id
WHERE c.host_id = $1
  AND c.deleted_at IS NULL AND p.deleted_at IS NULL
ORDER BY p.id;

-- name: GetProject :one
SELECT id, user_id, name, created_at, updated_at, deleted_at
FROM projects
WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL;

-- name: UpdateProject :one
UPDATE projects
SET name = $3, updated_at = NOW()
WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
RETURNING id, user_id, name, created_at, updated_at, deleted_at;

-- name: SoftDeleteProject :one
UPDATE projects
SET deleted_at = NOW(), updated_at = NOW()
WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
RETURNING id;
