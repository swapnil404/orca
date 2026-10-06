import type { ProjectClusterState } from '../types/resources'

export function replicaRemovalProgress(replicaID: string, revision: string | undefined, state: ProjectClusterState | undefined, now: number): { phase: 'waiting' | 'complete' | 'failed'; message: string } {
  const waiting = { phase: 'waiting' as const, message: 'Removing replica · waiting for agent confirmation' }
  if (!state || state.stale || !state.last_seen || now - Date.parse(state.last_seen) > 120_000 || !Number.isFinite(Date.parse(state.last_seen)) || !revision || !state.desired_state_revision) return waiting
  try { if (BigInt(state.desired_state_revision) < BigInt(revision)) return waiting } catch { return waiting }
  const failure = state.reconciliation_results.find((result) => result.action === 'delete_replica' && result.status !== 'success')
  if (failure) return { phase: 'failed', message: `Replica removal failed: ${failure.error || 'Agent could not finish removal'}` }
  if (!state.actual_state || state.actual_state.replicas?.some((replica) => replica.id === replicaID)) return waiting
  return { phase: 'complete', message: 'Replica removed · confirmed by agent' }
}
