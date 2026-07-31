-- name: CreateCluster :one
INSERT INTO clusters (
    id, project_id, host_id, name, postgres_version, parameters,
    replica_count, replica_ids, enabled_extensions, pgbouncer_enabled, pgbouncer_pool_mode,
    pgbouncer_max_connections, pgbackrest_enabled, pgbackrest_repo_path,
    pgbackrest_retention_full, pgbackrest_retention_diff,
    pgbackrest_full_interval_seconds, pgbackrest_diff_interval_seconds,
    pgbackrest_incr_interval_seconds, pg_hba_rules
)
SELECT sqlc.arg(cluster_id)::text, p.id, h.id, sqlc.arg(name)::text,
       sqlc.arg(postgres_version)::text, sqlc.arg(parameters)::jsonb,
       sqlc.arg(replica_count)::integer, sqlc.arg(replica_ids)::jsonb, sqlc.arg(enabled_extensions)::jsonb,
       sqlc.arg(pgbouncer_enabled)::boolean, sqlc.arg(pgbouncer_pool_mode)::text,
       sqlc.arg(pgbouncer_max_connections)::integer,
       sqlc.arg(pgbackrest_enabled)::boolean, sqlc.arg(pgbackrest_repo_path)::text,
       sqlc.arg(pgbackrest_retention_full)::integer, sqlc.arg(pgbackrest_retention_diff)::integer,
       sqlc.arg(pgbackrest_full_interval_seconds)::bigint, sqlc.arg(pgbackrest_diff_interval_seconds)::bigint,
       sqlc.arg(pgbackrest_incr_interval_seconds)::bigint, sqlc.arg(pg_hba_rules)::jsonb
FROM projects p
JOIN hosts h ON h.id = sqlc.arg(host_id) AND h.user_id = sqlc.arg(user_id)
JOIN organization_memberships om
  ON om.organization_id = p.organization_id AND om.user_id = sqlc.arg(user_id)
WHERE p.id = sqlc.arg(project_id) AND p.deleted_at IS NULL
RETURNING id, project_id, host_id, name, postgres_version, parameters,
           replica_count, pgbouncer_enabled, created_at, updated_at, deleted_at,
          pgbackrest_enabled, pgbackrest_repo_path,
          pgbackrest_retention_full, pgbackrest_retention_diff,
          pgbackrest_full_interval_seconds, pgbackrest_diff_interval_seconds,
           pgbackrest_incr_interval_seconds, replica_ids, enabled_extensions,
             pgbouncer_pool_mode, pgbouncer_max_connections, pg_hba_rules, restart_generation;

-- name: ListClusters :many
SELECT c.id, c.project_id, c.host_id, c.name, c.postgres_version, c.parameters,
       c.replica_count, c.pgbouncer_enabled, c.created_at, c.updated_at, c.deleted_at,
       c.pgbackrest_enabled, c.pgbackrest_repo_path,
       c.pgbackrest_retention_full, c.pgbackrest_retention_diff,
       c.pgbackrest_full_interval_seconds, c.pgbackrest_diff_interval_seconds,
       c.pgbackrest_incr_interval_seconds, c.replica_ids, c.enabled_extensions,
         c.pgbouncer_pool_mode, c.pgbouncer_max_connections, c.pg_hba_rules, c.restart_generation
FROM clusters c
JOIN projects p ON p.id = c.project_id
JOIN organization_memberships om ON om.organization_id = p.organization_id
WHERE c.project_id = $1 AND om.user_id = $2
  AND c.deleted_at IS NULL AND p.deleted_at IS NULL
ORDER BY c.created_at, c.id;

-- name: GetCluster :one
SELECT c.id, c.project_id, c.host_id, c.name, c.postgres_version, c.parameters,
       c.replica_count, c.pgbouncer_enabled, c.created_at, c.updated_at, c.deleted_at,
       c.pgbackrest_enabled, c.pgbackrest_repo_path,
       c.pgbackrest_retention_full, c.pgbackrest_retention_diff,
       c.pgbackrest_full_interval_seconds, c.pgbackrest_diff_interval_seconds,
       c.pgbackrest_incr_interval_seconds, c.replica_ids, c.enabled_extensions,
         c.pgbouncer_pool_mode, c.pgbouncer_max_connections, c.pg_hba_rules, c.restart_generation
FROM clusters c
JOIN projects p ON p.id = c.project_id
JOIN organization_memberships om ON om.organization_id = p.organization_id
WHERE c.id = $1 AND om.user_id = $2
  AND c.deleted_at IS NULL AND p.deleted_at IS NULL;

-- name: UpdateCluster :one
UPDATE clusters c
SET name = $3,
    postgres_version = $4,
    parameters = $5,
    replica_count = $6,
    replica_ids = $7,
    enabled_extensions = $8,
    pgbouncer_enabled = $9,
    pgbouncer_pool_mode = $10,
    pgbouncer_max_connections = $11,
    pgbackrest_enabled = $12,
    pgbackrest_repo_path = $13,
    pgbackrest_retention_full = $14,
    pgbackrest_retention_diff = $15,
    pgbackrest_full_interval_seconds = $16,
    pgbackrest_diff_interval_seconds = $17,
    pgbackrest_incr_interval_seconds = $18,
    pg_hba_rules = $19,
    updated_at = NOW()
FROM projects p, organization_memberships om
WHERE c.id = $1 AND c.project_id = p.id
  AND om.organization_id = p.organization_id AND om.user_id = $2
  AND c.deleted_at IS NULL AND p.deleted_at IS NULL
RETURNING c.id, c.project_id, c.host_id, c.name, c.postgres_version, c.parameters,
           c.replica_count, c.pgbouncer_enabled, c.created_at, c.updated_at, c.deleted_at,
          c.pgbackrest_enabled, c.pgbackrest_repo_path,
          c.pgbackrest_retention_full, c.pgbackrest_retention_diff,
          c.pgbackrest_full_interval_seconds, c.pgbackrest_diff_interval_seconds,
           c.pgbackrest_incr_interval_seconds, c.replica_ids, c.enabled_extensions,
             c.pgbouncer_pool_mode, c.pgbouncer_max_connections, c.pg_hba_rules, c.restart_generation;

-- name: SoftDeleteCluster :one
UPDATE clusters c
SET deleted_at = NOW(), updated_at = NOW()
FROM projects p, organization_memberships om
WHERE c.id = $1 AND c.project_id = p.id
  AND om.organization_id = p.organization_id AND om.user_id = $2
  AND c.deleted_at IS NULL AND p.deleted_at IS NULL
RETURNING c.id, c.project_id, c.host_id;

-- name: UpdateClusterPgBouncer :one
UPDATE clusters c
SET pgbouncer_pool_mode = sqlc.arg(pgbouncer_pool_mode)::text,
    pgbouncer_max_connections = sqlc.arg(pgbouncer_max_connections)::integer,
    updated_at = NOW()
WHERE c.id = sqlc.arg(cluster_id) AND c.pgbouncer_enabled AND c.deleted_at IS NULL
RETURNING c.id, c.project_id, c.host_id, c.name, c.postgres_version, c.parameters,
          c.replica_count, c.pgbouncer_enabled, c.created_at, c.updated_at, c.deleted_at,
          c.pgbackrest_enabled, c.pgbackrest_repo_path,
          c.pgbackrest_retention_full, c.pgbackrest_retention_diff,
          c.pgbackrest_full_interval_seconds, c.pgbackrest_diff_interval_seconds,
          c.pgbackrest_incr_interval_seconds, c.replica_ids, c.enabled_extensions,
            c.pgbouncer_pool_mode, c.pgbouncer_max_connections, c.pg_hba_rules, c.restart_generation;

-- name: UpdateClusterPgHba :one
UPDATE clusters c
SET pg_hba_rules = sqlc.arg(pg_hba_rules)::jsonb,
    updated_at = NOW()
WHERE c.id = sqlc.arg(cluster_id) AND c.deleted_at IS NULL
RETURNING c.id, c.project_id, c.host_id, c.name, c.postgres_version, c.parameters,
          c.replica_count, c.pgbouncer_enabled, c.created_at, c.updated_at, c.deleted_at,
          c.pgbackrest_enabled, c.pgbackrest_repo_path,
          c.pgbackrest_retention_full, c.pgbackrest_retention_diff,
          c.pgbackrest_full_interval_seconds, c.pgbackrest_diff_interval_seconds,
          c.pgbackrest_incr_interval_seconds, c.replica_ids, c.enabled_extensions,
           c.pgbouncer_pool_mode, c.pgbouncer_max_connections, c.pg_hba_rules, c.restart_generation;

-- name: UpdateClusterParameters :one
UPDATE clusters c
SET parameters = sqlc.arg(parameters)::jsonb,
    updated_at = NOW()
FROM projects p, organization_memberships om
WHERE c.id = sqlc.arg(cluster_id) AND c.project_id = p.id
  AND om.organization_id = p.organization_id AND om.user_id = sqlc.arg(user_id)
  AND c.deleted_at IS NULL AND p.deleted_at IS NULL
RETURNING c.id, c.project_id, c.host_id, c.name, c.postgres_version, c.parameters,
          c.replica_count, c.pgbouncer_enabled, c.created_at, c.updated_at, c.deleted_at,
          c.pgbackrest_enabled, c.pgbackrest_repo_path,
          c.pgbackrest_retention_full, c.pgbackrest_retention_diff,
          c.pgbackrest_full_interval_seconds, c.pgbackrest_diff_interval_seconds,
          c.pgbackrest_incr_interval_seconds, c.replica_ids, c.enabled_extensions,
          c.pgbouncer_pool_mode, c.pgbouncer_max_connections, c.pg_hba_rules, c.restart_generation;

-- name: UpdateClusterRestart :one
UPDATE clusters c
SET restart_generation = restart_generation + 1,
    updated_at = NOW()
FROM projects p, organization_memberships om
WHERE c.id = sqlc.arg(cluster_id) AND c.project_id = p.id
  AND om.organization_id = p.organization_id AND om.user_id = sqlc.arg(user_id)
  AND c.deleted_at IS NULL AND p.deleted_at IS NULL
RETURNING c.id, c.project_id, c.host_id, c.name, c.postgres_version, c.parameters,
          c.replica_count, c.pgbouncer_enabled, c.created_at, c.updated_at, c.deleted_at,
          c.pgbackrest_enabled, c.pgbackrest_repo_path,
          c.pgbackrest_retention_full, c.pgbackrest_retention_diff,
          c.pgbackrest_full_interval_seconds, c.pgbackrest_diff_interval_seconds,
          c.pgbackrest_incr_interval_seconds, c.replica_ids, c.enabled_extensions,
          c.pgbouncer_pool_mode, c.pgbouncer_max_connections, c.pg_hba_rules, c.restart_generation;

-- name: ListActiveClustersForProject :many
SELECT c.id, c.project_id, c.host_id, c.name, c.postgres_version, c.parameters,
       c.replica_count, c.pgbouncer_enabled, c.created_at, c.updated_at, c.deleted_at,
       c.pgbackrest_enabled, c.pgbackrest_repo_path,
       c.pgbackrest_retention_full, c.pgbackrest_retention_diff,
       c.pgbackrest_full_interval_seconds, c.pgbackrest_diff_interval_seconds,
       c.pgbackrest_incr_interval_seconds, c.replica_ids, c.enabled_extensions,
         c.pgbouncer_pool_mode, c.pgbouncer_max_connections, c.pg_hba_rules, c.restart_generation
FROM clusters c
JOIN projects p ON p.id = c.project_id
JOIN organization_memberships om ON om.organization_id = p.organization_id
WHERE c.project_id = $1 AND om.user_id = $2
  AND c.deleted_at IS NULL AND p.deleted_at IS NULL
ORDER BY c.id;

-- name: SoftDeleteClustersForProject :exec
UPDATE clusters c
SET deleted_at = NOW(), updated_at = NOW()
FROM projects p, organization_memberships om
WHERE c.project_id = $1 AND c.project_id = p.id
  AND om.organization_id = p.organization_id AND om.user_id = $2
  AND c.deleted_at IS NULL AND p.deleted_at IS NULL;
