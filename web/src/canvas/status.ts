import type { ActualBackup, ActualCluster, ActualPgBouncer, ActualReplica, ProjectClusterState } from '../types/resources'

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
  if (state.health === 'down') return state.health
  const replicationHealthy =
    replica.standby_connected === true &&
    replica.streaming_state === 'streaming' &&
    replica.replication_lag_bytes !== undefined &&
    replica.replication_lag_status === 'known'
  return isRunning(replica.status) && replicationHealthy ? 'healthy' : 'degraded'
}

export function pgBouncerStatus(
  state: ProjectClusterState | undefined,
  pgBouncer: ActualPgBouncer | undefined,
  now = Date.now(),
): NodeStatus {
  if (!state) return 'unknown'
  if (isReportStale(state, now)) return 'stale'
  if (state.health === 'down') return state.health
  if (!pgBouncer) return 'unknown'
  return isRunning(pgBouncer.status) && pgBouncer.admin_console_reachable === true ? 'healthy' : 'degraded'
}

export function pgBackRestStatus(
  state: ProjectClusterState | undefined,
  backup: ActualBackup | undefined,
  now = Date.now(),
): NodeStatus {
  if (!state) return 'unknown'
  if (isReportStale(state, now)) return 'stale'
  if (state.health === 'down') return 'down'
  if (!backup) return 'unknown'
  if (backup.status === 'failed') return 'degraded'
  if (backup.status === 'pending') return 'pending'
  return backup.status === 'succeeded' ? 'healthy' : 'unknown'
}

export function isReportStale(state: ProjectClusterState, now = Date.now()): boolean {
  if (state.stale) return true
  return state.last_seen !== undefined && now - new Date(state.last_seen).getTime() > reportStalenessWindowMs
}

export function areReportsFresh(states: ProjectClusterState[], now = Date.now()): boolean {
  return states.length > 0 && states.every((state) => state.last_seen !== undefined && !isReportStale(state, now))
}

export function extensionStatus(state: ProjectClusterState | undefined, desired: string[], now = Date.now()): NodeStatus {
  const clusterStatus = primaryStatus(state, now)
  if (clusterStatus !== 'healthy') return clusterStatus
  const actual = state?.actual_state?.enabled_extensions
  if (!actual) return 'unknown'
  const expected = [...desired].sort()
  const reported = [...actual].sort()
  return expected.length === reported.length && expected.every((extension, index) => extension === reported[index]) ? 'healthy' : 'degraded'
}

export function extensionPresenceStatus(state: ProjectClusterState | undefined, extension: string, now = Date.now()): NodeStatus {
  const clusterStatus = primaryStatus(state, now)
  if (clusterStatus !== 'healthy') return clusterStatus
  const actual = state?.actual_state?.enabled_extensions
  if (!actual) return 'unknown'
  return actual.includes(extension) ? 'healthy' : 'degraded'
}

function isRunning(status: string | undefined): boolean {
  return status === 'running' || status === 'healthy'
}
