import type { ActualCluster, ActualPgBouncer, ActualReplica, ProjectClusterState } from '../types/resources'

export type NodeStatus = 'healthy' | 'degraded' | 'down' | 'pending' | 'stale' | 'unknown'

const reportStalenessWindowMs = 2 * 60 * 1_000

export function primaryStatus(state: ProjectClusterState | undefined, now = Date.now()): NodeStatus {
  if (!state) return 'unknown'
  if (isReportStale(state, now)) return 'stale'
  if (state.health !== 'healthy') return state.health
  if (!state.actual_state) return 'unknown'
  return isRunning(state.actual_state.status) ? 'healthy' : 'degraded'
}

export function replicaStatus(
  state: ProjectClusterState | undefined,
  replica: ActualReplica | undefined,
  now = Date.now(),
): NodeStatus {
  if (!state || !replica) return 'unknown'
  if (isReportStale(state, now)) return 'stale'
  if (state.health === 'down' || state.health === 'pending') return state.health
  const replicationHealthy =
    replica.standby_connected === true &&
    replica.streaming_state === 'streaming' &&
    !['degraded', 'lagging', 'critical'].includes(replica.replication_lag_status ?? '')
  return isRunning(replica.status) && replicationHealthy ? 'healthy' : 'degraded'
}

export function pgBouncerStatus(
  state: ProjectClusterState | undefined,
  pgBouncer: ActualPgBouncer | undefined,
  now = Date.now(),
): NodeStatus {
  if (!state || !pgBouncer) return 'unknown'
  if (isReportStale(state, now)) return 'stale'
  return isRunning(pgBouncer.status) ? 'healthy' : 'degraded'
}

function isReportStale(state: ProjectClusterState, now: number): boolean {
  if (state.stale) return true
  return state.last_seen !== undefined && now - new Date(state.last_seen).getTime() > reportStalenessWindowMs
}

function isRunning(status: string | undefined): boolean {
  return status === 'running' || status === 'healthy'
}
