import test from 'node:test'
import assert from 'node:assert/strict'
import { restartProgress } from '../src/lib/restart.ts'
import type { ProjectClusterState } from '../src/types/resources.ts'

const now = Date.parse('2026-10-06T14:00:00Z')
const target = { id: 'cluster', name: 'main', restart_generation: '9007199254740993', desired_revision: '42' }
function report(overrides: Partial<ProjectClusterState> = {}): ProjectClusterState {
  return { cluster_id: 'cluster', host_id: 'host', actual_state: { id: 'cluster', applied_restart_generation: target.restart_generation }, health: 'healthy', last_seen: new Date(now).toISOString(), stale: false, desired_state_revision: '42', reconciliation_results: [], parameter_convergence: 'converged', ...overrides }
}

test('healthy old reports cannot confirm a new restart', () => {
  assert.equal(restartProgress(target, report({ actual_state: { id: 'cluster', applied_restart_generation: '9007199254740992' } }), now).phase, 'waiting')
})
test('acknowledgement and healthy services confirm completion', () => {
  assert.equal(restartProgress(target, report(), now).phase, 'complete')
  assert.equal(restartProgress(target, report({ health: 'degraded' }), now).phase, 'recovering')
})
test('stale or missing telemetry never confirms completion', () => {
  assert.equal(restartProgress(target, undefined, now).phase, 'waiting')
  assert.equal(restartProgress(target, report({ stale: true }), now).phase, 'waiting')
  assert.equal(restartProgress(target, report({ last_seen: new Date(now - 120001).toISOString() }), now).phase, 'waiting')
  assert.equal(restartProgress(target, report({ last_seen: undefined }), now).phase, 'waiting')
})
test('only failure reports at the requested revision apply', () => {
  const failure = report({ actual_state: null, reconciliation_results: [{ cluster_id: 'cluster', action: 'restart_cluster', status: 'failed', error: 'primary readiness failed' }] })
  assert.equal(restartProgress(target, failure, now).phase, 'failed')
  assert.equal(restartProgress(target, failure, now).detail, 'primary readiness failed')
  assert.equal(restartProgress(target, { ...failure, desired_state_revision: '41' }, now).phase, 'waiting')
})
test('later applied generations also satisfy the request', () => {
  assert.equal(restartProgress(target, report({ actual_state: { id: 'cluster', applied_restart_generation: '9007199254740994' } }), now).phase, 'complete')
})
