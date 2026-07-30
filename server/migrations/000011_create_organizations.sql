CREATE TYPE organization_role AS ENUM ('owner', 'admin', 'member');

CREATE TABLE organizations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL CHECK (name <> ''),
    slug TEXT NOT NULL UNIQUE CHECK (slug <> ''),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    personal_owner_user_id TEXT UNIQUE REFERENCES users (id)
);

CREATE TABLE organization_memberships (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role organization_role NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, user_id)
);

CREATE INDEX organization_memberships_user_id_idx
    ON organization_memberships (user_id, organization_id);

ALTER TABLE projects ADD COLUMN organization_id UUID REFERENCES organizations (id);

-- Projects predate the users table and historically had no ownership foreign key.
INSERT INTO users (id)
SELECT DISTINCT p.user_id
FROM projects p
WHERE NOT EXISTS (SELECT 1 FROM users u WHERE u.id = p.user_id);

WITH user_emails AS (
    SELECT u.id,
           COALESCE(u.email, MIN(oi.provider_email), u.id) AS email
    FROM users u
    LEFT JOIN oauth_identities oi ON oi.user_id = u.id
    GROUP BY u.id, u.email
), personal_organizations AS (
    SELECT id,
           SPLIT_PART(email, '@', 1) AS local_part
    FROM user_emails
)
INSERT INTO organizations (name, slug, personal_owner_user_id)
SELECT local_part || '''s workspace',
       COALESCE(
           NULLIF(TRIM(BOTH '-' FROM REGEXP_REPLACE(LOWER(local_part), '[^a-z0-9]+', '-', 'g')), ''),
           'workspace'
       ) || '-' || ENCODE(CONVERT_TO(id, 'UTF8'), 'hex'),
       id
FROM personal_organizations;

INSERT INTO organization_memberships (organization_id, user_id, role)
SELECT id, personal_owner_user_id, 'owner'
FROM organizations
WHERE personal_owner_user_id IS NOT NULL;

UPDATE projects p
SET organization_id = o.id
FROM organizations o
WHERE o.personal_owner_user_id = p.user_id;

ALTER TABLE projects ALTER COLUMN organization_id SET NOT NULL;
CREATE INDEX projects_organization_id_idx
    ON projects (organization_id, created_at, id)
    WHERE deleted_at IS NULL;

ALTER TABLE organizations DROP COLUMN personal_owner_user_id;
