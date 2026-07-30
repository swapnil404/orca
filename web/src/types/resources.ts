export interface Project {
  id: string
  name: string
  created_at: string
  updated_at: string
}

export interface Cluster {
  id: string
  project_id: string
  host_id: string
  name: string
  postgres_version: string
  parameters: Record<string, string>
  replica_count: number
  replicas: Array<{ id: string }>
  enabled_extensions: string[]
  pgbouncer_enabled: boolean
  pg_bouncer: PgBouncerConfig
  pg_back_rest?: PgBackRestConfig
  created_at: string
  updated_at: string
  desired_revision?: string
}

export interface ClusterInput {
  host_id?: string
  name: string
  postgres_version: string
  parameters: Record<string, string>
  replica_count: number
  enabled_extensions: string[]
  pgbouncer_enabled: boolean
  pg_bouncer?: PgBouncerConfig
  pg_back_rest?: PgBackRestConfig
}

export type PgBouncerPoolMode = 'session' | 'transaction' | 'statement'

export interface PgBouncerConfig {
  pool_mode: PgBouncerPoolMode
  max_connections: number
}

export interface ProjectHost {
  id: string
  status: 'never_connected' | 'online' | 'offline'
  connected_at?: string
}

export interface HostRegistration {
  host_id: string
  status: 'never_connected'
  docker_run_command: string
}

export interface PgBackRestConfig {
  repo_path: string
  retention_full: number
  retention_diff: number
  full_interval_seconds: number
  diff_interval_seconds: number
  incr_interval_seconds: number
}

export interface ActualReplica {
  id: string
  container_id?: string
  status?: string
  replication_lag_bytes?: string | number
  standby_connected?: boolean
  streaming_state?: string
  last_wal_received_lsn?: string
  last_wal_replayed_lsn?: string
  replication_lag_status?: string
}

export interface ActualPgBouncer {
  container_id?: string
  status?: string
  config?: string
  active_client_connections?: number
  max_client_connections?: number
  admin_console_reachable?: boolean
}

export interface ActualBackup {
  last_success_unix_seconds?: number
  config: string
  size_bytes?: number
  status: string
}

export interface ActualCluster {
  id: string
  container_id?: string
  status?: string
  version?: string
  replicas?: ActualReplica[]
  pg_bouncer?: ActualPgBouncer
  backup?: ActualBackup
  enabled_extensions?: string[]
  extension_versions?: Record<string, string>
  extension_update_methods?: Record<string, 'hot_apply' | 'restart'>
  postgres_ready?: boolean
}

export type ClusterHealth = 'healthy' | 'degraded' | 'down' | 'pending' | 'unknown'

export interface ReconciliationResult {
  action: string
  cluster_id: string
  status: 'success' | 'failed' | 'skipped_due_to_dependency'
  error: string
}

export interface ProjectClusterState {
  cluster_id: string
  host_id: string
  actual_state: ActualCluster | null
  health: ClusterHealth
  last_seen?: string
  stale: boolean
  desired_state_revision?: string
  reconciliation_results: ReconciliationResult[]
}

export interface ProjectStateSnapshot {
  type: 'project_state'
  project_id: string
  desired_clusters: Cluster[]
  clusters: ProjectClusterState[]
}

export interface ProjectTopology {
  project: Project
  clusters: Cluster[]
}

export type BackupStatus = 'succeeded' | 'failed' | 'pending' | 'unknown' | 'not_configured'

export interface BackupJob {
  project_id: string
  project_name: string
  cluster_id: string
  cluster_name: string
  last_backup: string | null
  size_bytes: number | null
  pitr_enabled: boolean
  status: BackupStatus
}
