import type { ProjectClusterState } from '../types/resources'

export type NodeStatus = 'healthy' | 'degraded' | 'down' | 'pending' | 'stale' | 'unknown'

const reportStalenessWindowMs = 2 * 60 * 1_000

export function displayStatus(state: ProjectClusterState | undefined, now = Date.now()): NodeStatus {
  if (!state) return 'unknown'
  if (isReportStale(state, now)) return 'stale'
  return state.health
}

export function isReportStale(state: ProjectClusterState, now = Date.now()): boolean {
  if (state.stale) return true
  return state.last_seen !== undefined && now - new Date(state.last_seen).getTime() > reportStalenessWindowMs
}

export function areReportsFresh(states: ProjectClusterState[], now = Date.now()): boolean {
  return states.length > 0 && states.every((state) => state.last_seen !== undefined && !isReportStale(state, now))
}
