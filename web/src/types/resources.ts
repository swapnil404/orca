export interface Project {
  id: string
  organization_id: string
  name: string
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
  pg_hba_rules: PgHbaRule[]
  pgbouncer_enabled: boolean
  pg_bouncer: PgBouncerConfig
  pg_back_rest?: PgBackRestConfig
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
  pg_hba_rules: PgHbaRule[]
  pgbouncer_enabled: boolean
  pg_bouncer?: PgBouncerConfig
  pg_back_rest?: PgBackRestConfig
}

export type PgBouncerPoolMode = 'session' | 'transaction' | 'statement'

export interface PgBouncerConfig {
  pool_mode: PgBouncerPoolMode
  max_connections: number
  publish_address: string
  publish_port: number
}

export type PgHbaType = 'host' | 'hostssl' | 'local'
export type PgHbaMethod = 'trust' | 'md5' | 'scram-sha-256' | 'reject'

export interface PgHbaRule {
  type: PgHbaType
  database: string
  user: string
  address: string
  method: PgHbaMethod
}

export interface ProjectHost {
  id: string
  status: 'never_connected' | 'online' | 'offline'
}

export interface HostRegistration {
  host_id: string
  status: ProjectHost['status']
  docker_run_command: string
}

export interface PgBackRestConfig {
  repo_path: string
  retention_full: number
  retention_diff: number
  schedule?: BackupSchedule
}

export interface BackupSchedule {
  full_interval_seconds: number
  diff_interval_seconds: number
  incr_interval_seconds: number
}

export interface ActualReplica {
  id: string
  status?: string
  replication_lag_bytes?: string | number
  standby_connected?: boolean
  streaming_state?: string
  replication_lag_status?: 'unknown' | 'known' | 'lagging' | 'critical'
  pg_hba_rules?: PgHbaRule[]
  pg_hba_observed?: boolean
  applied_params?: Record<string, string>
  parameters_observed?: boolean
}

export interface ActualPgBouncer {
  container_id?: string
  status?: string
  config?: string
  active_client_connections?: number
  admin_console_reachable?: boolean
  network_name?: string
  published_address?: string
  published_port?: number
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
  pg_hba_rules?: PgHbaRule[]
  pg_hba_observed?: boolean
  network_cidrs?: string[]
  pg_hba_replication_cidrs?: string[]
  applied_params?: Record<string, string>
  parameter_states?: Record<string, PostgresParameterState>
  parameters_observed?: boolean
}

export interface PostgresParameterState {
  setting: string
  unit: string
  context: string
  update_method: 'reload' | 'restart' | ''
  pending_restart: boolean
  error: string
  applied: boolean
}

export type ClusterHealth = 'healthy' | 'degraded' | 'down' | 'unknown'
export type ParameterConvergence = 'unknown' | 'pending' | 'failed' | 'restart_pending' | 'converged'

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
  parameter_convergence: ParameterConvergence
}

export interface ProjectStateSnapshot {
  type: 'project_state'
  project_id: string
  desired_clusters: Cluster[]
  clusters: ProjectClusterState[]
  restore_operations: RestoreOperation[]
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

export type RestoreOperationMode = 'in_place' | 'clone'
export type RestoreOperationIntent = 'preflight' | 'execute' | 'cancel' | 'rollback' | 'finalize'
export type RestoreOperationStatus = 'pending' | 'ready' | 'running' | 'succeeded' | 'failed' | 'cancelled' | 'rolled_back' | 'finalized'

export interface RestoreOperationReport {
  phase: string
  error_code?: string
  error?: string
  destructive_started?: boolean
  cancellable?: boolean
  rollback_available?: boolean
  backup_label?: string
  recovery_earliest?: string
  recovery_latest?: string
  postgres_version?: string
  required_bytes?: number
  available_bytes?: number
}

export interface RestoreOperation {
  id: string
  source_cluster_id: string
  target_cluster_name?: string
  mode: RestoreOperationMode
  intent: RestoreOperationIntent
  status: RestoreOperationStatus
  target_time: string
  report?: RestoreOperationReport
  created_at: string
}

export type CreateRestoreOperationInput =
  | { mode: 'in_place'; target_time: string }
  | { mode: 'clone'; target_time: string; target_cluster_name: string }
