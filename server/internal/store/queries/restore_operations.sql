-- name: GetRestoreMutationContext :one
SELECT c.id AS source_cluster_id, c.project_id, c.host_id, c.pgbackrest_enabled,
       om.role, c.name, c.postgres_version, c.parameters, c.replica_count,
       c.replica_ids, c.enabled_extensions, c.pgbouncer_enabled,
       c.pgbouncer_pool_mode, c.pgbouncer_max_connections,
       c.pgbouncer_publish_address, c.pgbouncer_publish_port,
       c.pgbackrest_repo_path, c.pgbackrest_retention_full,
       c.pgbackrest_retention_diff, c.pgbackrest_full_interval_seconds,
       c.pgbackrest_diff_interval_seconds, c.pgbackrest_incr_interval_seconds,
       c.pg_hba_rules, c.restart_generation
FROM clusters c
JOIN projects p ON p.id = c.project_id AND p.deleted_at IS NULL
JOIN organization_memberships om ON om.organization_id = p.organization_id
WHERE c.id = sqlc.arg(source_cluster_id) AND om.user_id = sqlc.arg(user_id)
  AND c.deleted_at IS NULL
FOR UPDATE OF c;

-- name: FindRestoreOperationByIdempotencyKey :one
SELECT ro.*
FROM restore_operations ro
JOIN projects p ON p.id = ro.project_id
JOIN organization_memberships om ON om.organization_id = p.organization_id
WHERE ro.requested_by_user_id = sqlc.arg(user_id)
  AND ro.idempotency_key = sqlc.arg(idempotency_key)
  AND om.user_id = sqlc.arg(user_id)
  AND om.role IN ('owner', 'admin');

-- name: LockRestoreResource :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(resource_id), 0));

-- name: HasActiveRestoreConflict :one
SELECT EXISTS (
    SELECT 1 FROM restore_operations ro
    WHERE ro.status NOT IN ('cancelled', 'finalized')
      AND NOT (ro.mode = 'clone' AND ro.status = 'succeeded')
      AND (ro.source_cluster_id IN (sqlc.arg(source_cluster_id), sqlc.narg(target_cluster_id))
           OR ro.target_cluster_id IN (sqlc.arg(source_cluster_id), sqlc.narg(target_cluster_id)))
)::boolean;

-- name: HasClusterRestoreMutationConflict :one
SELECT EXISTS (
    SELECT 1 FROM restore_operations ro
    WHERE ro.status NOT IN ('cancelled', 'finalized')
      AND (ro.source_cluster_id = sqlc.arg(cluster_id)
           OR ro.target_cluster_id = sqlc.arg(cluster_id))
)::boolean;

-- name: HasProjectRestoreMutationConflict :one
SELECT EXISTS (
    SELECT 1
    FROM restore_operations ro
    WHERE ro.status NOT IN ('cancelled', 'finalized')
      AND EXISTS (
          SELECT 1
          FROM clusters c
          WHERE c.project_id = sqlc.arg(project_id)
            AND c.deleted_at IS NULL
            AND c.id IN (ro.source_cluster_id, ro.target_cluster_id)
      )
)::boolean;

-- name: CreateRestoreOperation :one
INSERT INTO restore_operations (
    id, project_id, host_id, source_cluster_id, target_cluster_id,
    target_cluster_name, target_spec, mode, target_time, request_fingerprint,
    idempotency_key, requested_by_user_id
) VALUES (
    sqlc.arg(id), sqlc.arg(project_id), sqlc.arg(host_id), sqlc.arg(source_cluster_id),
    sqlc.narg(target_cluster_id), sqlc.narg(target_cluster_name), sqlc.arg(target_spec),
    sqlc.arg(mode), sqlc.arg(target_time), sqlc.arg(request_fingerprint),
    sqlc.arg(idempotency_key), sqlc.arg(requested_by_user_id)
)
RETURNING *;

-- name: GetRestoreOperationForUpdate :one
SELECT sqlc.embed(ro), om.role
FROM restore_operations ro
JOIN projects p ON p.id = ro.project_id AND p.deleted_at IS NULL
JOIN organization_memberships om ON om.organization_id = p.organization_id
WHERE ro.id = sqlc.arg(id) AND om.user_id = sqlc.arg(user_id)
FOR UPDATE OF ro;

-- name: GetRestoreOperation :one
SELECT ro.*
FROM restore_operations ro
JOIN projects p ON p.id = ro.project_id AND p.deleted_at IS NULL
JOIN organization_memberships om ON om.organization_id = p.organization_id
WHERE ro.id = sqlc.arg(id) AND om.user_id = sqlc.arg(user_id);

-- name: ListRestoreOperations :many
SELECT ro.*
FROM restore_operations ro
JOIN projects p ON p.id = ro.project_id AND p.deleted_at IS NULL
JOIN organization_memberships om ON om.organization_id = p.organization_id
WHERE ro.project_id = sqlc.arg(project_id) AND om.user_id = sqlc.arg(user_id)
ORDER BY ro.created_at DESC, ro.id;

-- name: ListActionableRestoreOperationsForHost :many
SELECT * FROM restore_operations
WHERE host_id = sqlc.arg(host_id)
  AND status NOT IN ('cancelled', 'finalized')
ORDER BY created_at, id;

-- name: UpdateRestoreIntent :one
UPDATE restore_operations
SET intent = sqlc.arg(intent),
    updated_at = NOW()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: RequestRestoreFinalization :one
UPDATE restore_operations
SET intent = 'finalize',
    updated_at = NOW()
WHERE id = sqlc.arg(id) AND status IN ('succeeded', 'rolled_back')
RETURNING *;

-- name: LockRestoreOperationForAgentReport :one
SELECT * FROM restore_operations
WHERE id = sqlc.arg(id) AND host_id = sqlc.arg(host_id)
FOR UPDATE;

-- name: ApplyRestoreOperationReport :one
UPDATE restore_operations
SET report_sequence = sqlc.arg(report_sequence),
    status = sqlc.arg(status)::restore_operation_status,
    report = sqlc.arg(report),
    finalized_at = CASE
        WHEN sqlc.arg(status)::restore_operation_status = 'finalized'::restore_operation_status THEN NOW()
        ELSE finalized_at
    END,
    updated_at = NOW()
WHERE id = sqlc.arg(id) AND report_sequence < sqlc.arg(report_sequence)
RETURNING *;

-- name: CreateRestoreOperationEvent :one
INSERT INTO restore_operation_events (operation_id, host_id, event_type, payload)
VALUES (sqlc.arg(operation_id), sqlc.arg(host_id), sqlc.arg(event_type), sqlc.arg(payload))
RETURNING id;

-- name: GetRestoreRevisionForHost :one
SELECT COALESCE(MAX(id), 0)::BIGINT
FROM restore_operation_events
WHERE host_id = sqlc.arg(host_id);

-- name: CreateActivatedCloneCluster :one
INSERT INTO clusters (
    id, project_id, host_id, name, postgres_version, parameters,
    replica_count, replica_ids, enabled_extensions, pgbouncer_enabled,
    pgbouncer_pool_mode, pgbouncer_max_connections, pgbouncer_publish_address,
    pgbouncer_publish_port, pgbackrest_enabled,
    pgbackrest_repo_path, pgbackrest_retention_full, pgbackrest_retention_diff,
    pgbackrest_full_interval_seconds, pgbackrest_diff_interval_seconds,
    pgbackrest_incr_interval_seconds, pg_hba_rules, restart_generation
) VALUES (
    sqlc.arg(id), sqlc.arg(project_id), sqlc.arg(host_id), sqlc.arg(name),
    sqlc.arg(postgres_version), sqlc.arg(parameters), sqlc.arg(replica_count),
    sqlc.arg(replica_ids), sqlc.arg(enabled_extensions), sqlc.arg(pgbouncer_enabled),
    sqlc.arg(pgbouncer_pool_mode), sqlc.arg(pgbouncer_max_connections),
    sqlc.arg(pgbouncer_publish_address), sqlc.arg(pgbouncer_publish_port),
    sqlc.arg(pgbackrest_enabled), sqlc.arg(pgbackrest_repo_path),
    sqlc.arg(pgbackrest_retention_full), sqlc.arg(pgbackrest_retention_diff),
    sqlc.arg(pgbackrest_full_interval_seconds), sqlc.arg(pgbackrest_diff_interval_seconds),
    sqlc.arg(pgbackrest_incr_interval_seconds), sqlc.arg(pg_hba_rules),
    sqlc.arg(restart_generation)
)
RETURNING *;
