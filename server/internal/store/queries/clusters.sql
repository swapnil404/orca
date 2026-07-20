-- name: CreateCluster :one
INSERT INTO clusters (
    id, project_id, host_id, name, postgres_version, parameters,
    replica_count, pgbouncer_enabled
)
SELECT sqlc.arg(cluster_id)::text, p.id, h.id, sqlc.arg(name)::text,
       sqlc.arg(postgres_version)::text, sqlc.arg(parameters)::jsonb,
       sqlc.arg(replica_count)::integer, sqlc.arg(pgbouncer_enabled)::boolean
FROM projects p
JOIN hosts h ON h.id = sqlc.arg(host_id) AND h.user_id = sqlc.arg(user_id)
WHERE p.id = sqlc.arg(project_id) AND p.user_id = sqlc.arg(user_id) AND p.deleted_at IS NULL
RETURNING id, project_id, host_id, name, postgres_version, parameters,
          replica_count, pgbouncer_enabled, created_at, updated_at, deleted_at;

-- name: ListClusters :many
SELECT c.id, c.project_id, c.host_id, c.name, c.postgres_version, c.parameters,
       c.replica_count, c.pgbouncer_enabled, c.created_at, c.updated_at, c.deleted_at
FROM clusters c
JOIN projects p ON p.id = c.project_id
WHERE c.project_id = $1 AND p.user_id = $2
  AND c.deleted_at IS NULL AND p.deleted_at IS NULL
ORDER BY c.created_at, c.id;

-- name: GetCluster :one
SELECT c.id, c.project_id, c.host_id, c.name, c.postgres_version, c.parameters,
       c.replica_count, c.pgbouncer_enabled, c.created_at, c.updated_at, c.deleted_at
FROM clusters c
JOIN projects p ON p.id = c.project_id
WHERE c.id = $1 AND p.user_id = $2
  AND c.deleted_at IS NULL AND p.deleted_at IS NULL;

-- name: UpdateCluster :one
UPDATE clusters c
SET name = $3,
    postgres_version = $4,
    parameters = $5,
    replica_count = $6,
    pgbouncer_enabled = $7,
    updated_at = NOW()
FROM projects p
WHERE c.id = $1 AND c.project_id = p.id AND p.user_id = $2
  AND c.deleted_at IS NULL AND p.deleted_at IS NULL
RETURNING c.id, c.project_id, c.host_id, c.name, c.postgres_version, c.parameters,
          c.replica_count, c.pgbouncer_enabled, c.created_at, c.updated_at, c.deleted_at;

-- name: SoftDeleteCluster :one
UPDATE clusters c
SET deleted_at = NOW(), updated_at = NOW()
FROM projects p
WHERE c.id = $1 AND c.project_id = p.id AND p.user_id = $2
  AND c.deleted_at IS NULL AND p.deleted_at IS NULL
RETURNING c.id, c.project_id, c.host_id;

-- name: ListActiveClustersForProject :many
SELECT c.id, c.project_id, c.host_id, c.name, c.postgres_version, c.parameters,
       c.replica_count, c.pgbouncer_enabled, c.created_at, c.updated_at, c.deleted_at
FROM clusters c
JOIN projects p ON p.id = c.project_id
WHERE c.project_id = $1 AND p.user_id = $2
  AND c.deleted_at IS NULL AND p.deleted_at IS NULL
ORDER BY c.id;

-- name: SoftDeleteClustersForProject :exec
UPDATE clusters c
SET deleted_at = NOW(), updated_at = NOW()
FROM projects p
WHERE c.project_id = $1 AND c.project_id = p.id AND p.user_id = $2
  AND c.deleted_at IS NULL AND p.deleted_at IS NULL;
