import test from 'node:test'
import assert from 'node:assert/strict'
import { replicaRemovalProgress } from '../src/lib/replica-removal.ts'
import type { ProjectClusterState } from '../src/types/resources.ts'
const now = Date.now()
const state: ProjectClusterState = { cluster_id: 'cluster', host_id: 'host', actual_state: { id: 'cluster', replicas: [{ id: 'keep' }] }, health: 'healthy', last_seen: new Date(now).toISOString(), stale: false, desired_state_revision: '43', reconciliation_results: [], parameter_convergence: 'converged' }
test('removal requires a current report of the requested state', () => {
 assert.equal(replicaRemovalProgress('selected', '43', state, now).phase, 'complete')
 assert.equal(replicaRemovalProgress('selected', '44', state, now).phase, 'waiting')
 assert.equal(replicaRemovalProgress('selected', '43', { ...state, stale: true }, now).phase, 'waiting')
 assert.equal(replicaRemovalProgress('selected', '43', { ...state, actual_state: { id: 'cluster', replicas: [{id:'selected'}] } }, now).phase, 'waiting')
})
test('cleanup failures cannot become success just because the container disappeared', () => {
 const failed = { ...state, reconciliation_results: [{ action:'delete_replica', cluster_id:'cluster', status:'failed' as const, error:'' }] }
 assert.equal(replicaRemovalProgress('selected', '43', failed, now).phase, 'failed')
})
