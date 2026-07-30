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
SELECT o.id, o.name, o.slug, o.created_at
FROM organizations o
JOIN organization_memberships requester
  ON requester.organization_id = o.id
 AND requester.user_id = sqlc.arg(user_id)
WHERE o.id = sqlc.arg(organization_id);

-- name: GetOrganizationBySlug :one
SELECT id, name, slug, created_at
FROM organizations
WHERE slug = $1;

-- name: UpdateOrganization :one
UPDATE organizations o
SET name = sqlc.arg(name)::text
WHERE o.id = sqlc.arg(organization_id)
  AND EXISTS (
      SELECT 1
      FROM organization_memberships om
      WHERE om.organization_id = o.id
        AND om.user_id = sqlc.arg(user_id)
        AND om.role = 'owner'
  )
RETURNING id, name, slug, created_at;

-- name: GetOrganizationDeletionState :one
SELECT om.role,
       EXISTS (SELECT 1 FROM projects p WHERE p.organization_id = o.id) AS has_projects
FROM organizations o
JOIN organization_memberships om ON om.organization_id = o.id
WHERE o.id = sqlc.arg(organization_id)
  AND om.user_id = sqlc.arg(user_id)
FOR UPDATE OF o;

-- name: DeleteOrganization :execrows
DELETE FROM organizations
WHERE id = $1;

-- name: ListOrganizationsForUser :many
SELECT o.id, o.name, o.slug, o.created_at
FROM organizations o
JOIN organization_memberships om ON om.organization_id = o.id
WHERE om.user_id = $1
ORDER BY o.created_at, o.id;

-- name: ListMembersForOrganization :many
SELECT om.id, om.organization_id, om.user_id, om.role, om.created_at,
       COALESCE(u.email, oi.provider_email) AS email
FROM organization_memberships om
JOIN organization_memberships requester
  ON requester.organization_id = om.organization_id
 AND requester.user_id = sqlc.arg(user_id)
JOIN users u ON u.id = om.user_id
LEFT JOIN LATERAL (
    SELECT provider_email
    FROM oauth_identities
    WHERE user_id = u.id AND provider_email IS NOT NULL
    ORDER BY created_at, provider, provider_user_id
    LIMIT 1
) oi ON TRUE
WHERE om.organization_id = sqlc.arg(organization_id)
  AND u.deleted_at IS NULL
ORDER BY om.created_at, om.id;

-- name: ListProjectsForOrganization :many
WITH authorized AS (
    SELECT om.organization_id
    FROM organization_memberships om
    WHERE om.organization_id = sqlc.arg(organization_id)
      AND om.user_id = sqlc.arg(user_id)
)
SELECT CASE WHEN p.id IS NULL THEN FALSE ELSE TRUE END AS has_project,
       COALESCE(p.id, '') AS id,
       COALESCE(p.name, '') AS name,
       COALESCE(p.created_at, TIMESTAMPTZ 'epoch') AS created_at,
       COALESCE(p.updated_at, TIMESTAMPTZ 'epoch') AS updated_at,
       a.organization_id
FROM authorized a
LEFT JOIN projects p
  ON p.organization_id = a.organization_id
 AND p.deleted_at IS NULL
ORDER BY p.created_at, p.id;

-- name: GetMembershipForUserAndOrg :one
SELECT id, organization_id, user_id, role, created_at
FROM organization_memberships
WHERE organization_id = $1 AND user_id = $2;
