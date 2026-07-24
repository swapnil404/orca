-- name: CreateCluster :one
INSERT INTO clusters (
    id, project_id, host_id, name, postgres_version, parameters,
    replica_count, pgbouncer_enabled, pgbackrest_enabled, pgbackrest_repo_path,
    pgbackrest_retention_full, pgbackrest_retention_diff,
    pgbackrest_full_interval_seconds, pgbackrest_diff_interval_seconds,
    pgbackrest_incr_interval_seconds
)
SELECT sqlc.arg(cluster_id)::text, p.id, h.id, sqlc.arg(name)::text,
       sqlc.arg(postgres_version)::text, sqlc.arg(parameters)::jsonb,
       sqlc.arg(replica_count)::integer, sqlc.arg(pgbouncer_enabled)::boolean,
       sqlc.arg(pgbackrest_enabled)::boolean, sqlc.arg(pgbackrest_repo_path)::text,
       sqlc.arg(pgbackrest_retention_full)::integer, sqlc.arg(pgbackrest_retention_diff)::integer,
       sqlc.arg(pgbackrest_full_interval_seconds)::bigint, sqlc.arg(pgbackrest_diff_interval_seconds)::bigint,
       sqlc.arg(pgbackrest_incr_interval_seconds)::bigint
FROM projects p
JOIN hosts h ON h.id = sqlc.arg(host_id) AND h.user_id = sqlc.arg(user_id)
WHERE p.id = sqlc.arg(project_id) AND p.user_id = sqlc.arg(user_id) AND p.deleted_at IS NULL
RETURNING id, project_id, host_id, name, postgres_version, parameters,
          replica_count, pgbouncer_enabled, created_at, updated_at, deleted_at,
          pgbackrest_enabled, pgbackrest_repo_path,
          pgbackrest_retention_full, pgbackrest_retention_diff,
          pgbackrest_full_interval_seconds, pgbackrest_diff_interval_seconds,
          pgbackrest_incr_interval_seconds;

-- name: ListClusters :many
SELECT c.id, c.project_id, c.host_id, c.name, c.postgres_version, c.parameters,
       c.replica_count, c.pgbouncer_enabled, c.created_at, c.updated_at, c.deleted_at,
       c.pgbackrest_enabled, c.pgbackrest_repo_path,
       c.pgbackrest_retention_full, c.pgbackrest_retention_diff,
       c.pgbackrest_full_interval_seconds, c.pgbackrest_diff_interval_seconds,
       c.pgbackrest_incr_interval_seconds
FROM clusters c
JOIN projects p ON p.id = c.project_id
WHERE c.project_id = $1 AND p.user_id = $2
  AND c.deleted_at IS NULL AND p.deleted_at IS NULL
ORDER BY c.created_at, c.id;

-- name: GetCluster :one
SELECT c.id, c.project_id, c.host_id, c.name, c.postgres_version, c.parameters,
       c.replica_count, c.pgbouncer_enabled, c.created_at, c.updated_at, c.deleted_at,
       c.pgbackrest_enabled, c.pgbackrest_repo_path,
       c.pgbackrest_retention_full, c.pgbackrest_retention_diff,
       c.pgbackrest_full_interval_seconds, c.pgbackrest_diff_interval_seconds,
       c.pgbackrest_incr_interval_seconds
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
    pgbackrest_enabled = $8,
    pgbackrest_repo_path = $9,
    pgbackrest_retention_full = $10,
    pgbackrest_retention_diff = $11,
    pgbackrest_full_interval_seconds = $12,
    pgbackrest_diff_interval_seconds = $13,
    pgbackrest_incr_interval_seconds = $14,
    updated_at = NOW()
FROM projects p
WHERE c.id = $1 AND c.project_id = p.id AND p.user_id = $2
  AND c.deleted_at IS NULL AND p.deleted_at IS NULL
RETURNING c.id, c.project_id, c.host_id, c.name, c.postgres_version, c.parameters,
          c.replica_count, c.pgbouncer_enabled, c.created_at, c.updated_at, c.deleted_at,
          c.pgbackrest_enabled, c.pgbackrest_repo_path,
          c.pgbackrest_retention_full, c.pgbackrest_retention_diff,
          c.pgbackrest_full_interval_seconds, c.pgbackrest_diff_interval_seconds,
          c.pgbackrest_incr_interval_seconds;

-- name: SoftDeleteCluster :one
UPDATE clusters c
SET deleted_at = NOW(), updated_at = NOW()
FROM projects p
WHERE c.id = $1 AND c.project_id = p.id AND p.user_id = $2
  AND c.deleted_at IS NULL AND p.deleted_at IS NULL
RETURNING c.id, c.project_id, c.host_id;

-- name: ListActiveClustersForProject :many
SELECT c.id, c.project_id, c.host_id, c.name, c.postgres_version, c.parameters,
       c.replica_count, c.pgbouncer_enabled, c.created_at, c.updated_at, c.deleted_at,
       c.pgbackrest_enabled, c.pgbackrest_repo_path,
       c.pgbackrest_retention_full, c.pgbackrest_retention_diff,
       c.pgbackrest_full_interval_seconds, c.pgbackrest_diff_interval_seconds,
       c.pgbackrest_incr_interval_seconds
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
