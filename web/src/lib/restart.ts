import type { Cluster, ProjectClusterState } from '../types/resources'

export type RestartPhase = 'waiting' | 'recovering' | 'complete' | 'failed'
export type RestartTarget = Pick<Cluster, 'id' | 'name' | 'restart_generation' | 'desired_revision'>
export interface RestartProgressRow { id: string; name: string; phase: RestartPhase; detail: string }

function atLeast(actual: string | number | undefined, desired: string | number | undefined): boolean {
  if (actual === undefined || desired === undefined) return false
  try { return BigInt(actual) >= BigInt(desired) } catch { return false }
}

export function restartProgress(target: RestartTarget, state: ProjectClusterState | undefined, now: number): RestartProgressRow {
  const row = { id: target.id, name: target.name }
  const fresh = state !== undefined && !state.stale && state.last_seen !== undefined && now - new Date(state.last_seen).getTime() <= 120_000
  if (!fresh || !state) return { ...row, phase: 'waiting', detail: 'Waiting for a fresh agent report' }
  const applied = atLeast(state.actual_state?.applied_restart_generation, target.restart_generation)
  if (applied) {
    return state.health === 'healthy'
      ? { ...row, phase: 'complete', detail: 'Restart confirmed · healthy' }
      : { ...row, phase: 'recovering', detail: 'Restart confirmed · waiting for healthy services' }
  }
  const currentRevision = atLeast(state.desired_state_revision, target.desired_revision)
  const failure = currentRevision ? state.reconciliation_results.find((result) => result.action === 'restart_cluster' && result.status !== 'success') : undefined
  if (failure) return { ...row, phase: 'failed', detail: failure.error || 'The agent could not restart this cluster' }
  return { ...row, phase: 'waiting', detail: 'Request saved · waiting for agent acknowledgement' }
}
