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
  pgbouncer_enabled: boolean
  created_at: string
  updated_at: string
}

export interface ClusterInput {
  host_id?: string
  name: string
  postgres_version: string
  parameters: Record<string, string>
  replica_count: number
  pgbouncer_enabled: boolean
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
}

export interface ActualCluster {
  id: string
  container_id?: string
  status?: string
  version?: string
  replicas?: ActualReplica[]
  pg_bouncer?: ActualPgBouncer
}

export type ClusterHealth = 'healthy' | 'degraded' | 'down' | 'pending' | 'unknown'

export interface ProjectClusterState {
  cluster_id: string
  host_id: string
  actual_state: ActualCluster | null
  health: ClusterHealth
  last_seen?: string
  stale: boolean
}

export interface ProjectStateSnapshot {
  type: 'project_state'
  project_id: string
  clusters: ProjectClusterState[]
}

export interface ProjectTopology {
  project: Project
  clusters: Cluster[]
}
