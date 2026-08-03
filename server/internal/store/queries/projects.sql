-- name: CreateProject :one
INSERT INTO projects (id, user_id, organization_id, name)
SELECT sqlc.arg(id)::text, sqlc.arg(user_id)::text, om.organization_id, sqlc.arg(name)::text
FROM organization_memberships om
WHERE om.organization_id = sqlc.arg(organization_id)
  AND om.user_id = sqlc.arg(user_id)
RETURNING id, user_id, name, created_at, updated_at, deleted_at, organization_id;

-- name: ListProjects :many
SELECT p.id, p.user_id, p.name, p.created_at, p.updated_at, p.deleted_at, p.organization_id
FROM projects p
JOIN organization_memberships om ON om.organization_id = p.organization_id
WHERE om.user_id = $1 AND p.deleted_at IS NULL
ORDER BY p.created_at, p.id;

-- name: ListProjectIDsForHost :many
SELECT DISTINCT p.id
FROM projects p
JOIN clusters c ON c.project_id = p.id
WHERE c.host_id = $1
  AND c.deleted_at IS NULL AND p.deleted_at IS NULL
ORDER BY p.id;

-- name: GetProject :one
SELECT p.id, p.user_id, p.name, p.created_at, p.updated_at, p.deleted_at, p.organization_id
FROM projects p
JOIN organization_memberships om ON om.organization_id = p.organization_id
WHERE p.id = $1 AND om.user_id = $2 AND p.deleted_at IS NULL;

-- name: GetProjectMutationResource :one
SELECT p.id, om.role
FROM projects p
JOIN organization_memberships om ON om.organization_id = p.organization_id
WHERE p.id = sqlc.arg(project_id) AND om.user_id = sqlc.arg(user_id)
  AND p.deleted_at IS NULL
FOR UPDATE OF p;

-- name: UpdateProject :one
UPDATE projects p
SET name = $3, updated_at = NOW()
WHERE p.id = $1
  AND p.organization_id IN (
      SELECT om.organization_id FROM organization_memberships om WHERE om.user_id = $2
  )
  AND p.deleted_at IS NULL
RETURNING p.id, p.user_id, p.name, p.created_at, p.updated_at, p.deleted_at, p.organization_id;

-- name: SoftDeleteProject :one
UPDATE projects p
SET deleted_at = NOW(), updated_at = NOW()
WHERE p.id = $1
  AND p.organization_id IN (
      SELECT om.organization_id FROM organization_memberships om WHERE om.user_id = $2
  )
  AND p.deleted_at IS NULL
RETURNING p.id;
