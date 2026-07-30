-- name: CreateOrganization :one
WITH generated AS (
    SELECT gen_random_uuid() AS id
), values_to_insert AS (
    SELECT id,
           sqlc.arg(name)::text AS name,
           COALESCE(
               NULLIF(TRIM(BOTH '-' FROM REGEXP_REPLACE(LOWER(sqlc.arg(name)::text), '[^a-z0-9]+', '-', 'g')), ''),
               'workspace'
           ) || '-' || REPLACE(id::text, '-', '') AS slug
    FROM generated
)
INSERT INTO organizations (id, name, slug)
SELECT id, name, slug
FROM values_to_insert
RETURNING id, name, slug, created_at;

-- name: CreateMembership :one
INSERT INTO organization_memberships (organization_id, user_id, role)
VALUES (sqlc.arg(organization_id), sqlc.arg(user_id), sqlc.arg(role))
RETURNING id, organization_id, user_id, role, created_at;

-- name: GetOrganizationByID :one
SELECT id, name, slug, created_at
FROM organizations
WHERE id = $1;

-- name: GetOrganizationBySlug :one
SELECT id, name, slug, created_at
FROM organizations
WHERE slug = $1;

-- name: ListOrganizationsForUser :many
SELECT o.id, o.name, o.slug, o.created_at
FROM organizations o
JOIN organization_memberships om ON om.organization_id = o.id
WHERE om.user_id = $1
ORDER BY o.created_at, o.id;

-- name: ListMembersForOrganization :many
SELECT om.id, om.organization_id, om.user_id, om.role, om.created_at, u.email
FROM organization_memberships om
JOIN users u ON u.id = om.user_id
WHERE om.organization_id = $1
  AND u.deleted_at IS NULL
ORDER BY om.created_at, om.id;

-- name: ListProjectsForOrganization :many
SELECT id, user_id, name, created_at, updated_at, deleted_at, organization_id
FROM projects
WHERE organization_id = $1 AND deleted_at IS NULL
ORDER BY created_at, id;

-- name: GetMembershipForUserAndOrg :one
SELECT id, organization_id, user_id, role, created_at
FROM organization_memberships
WHERE organization_id = $1 AND user_id = $2;
