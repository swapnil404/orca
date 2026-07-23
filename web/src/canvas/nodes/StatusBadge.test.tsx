import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { ProjectClusterState } from '../../types/resources'
import { primaryStatus } from '../status'
import { StatusBadge } from './StatusBadge'

interface FixtureProps {
  state?: ProjectClusterState
}

function FixtureNode({ state }: FixtureProps) {
  return <StatusBadge status={primaryStatus(state)} />
}

const baseState: ProjectClusterState = {
  cluster_id: 'cluster-1',
  host_id: 'host-1',
  actual_state: { id: 'cluster-1', status: 'running', version: '16' },
  health: 'healthy',
  stale: false,
}

describe('node status rendering', () => {
  it.each([
    { name: 'healthy', state: baseState, expected: 'healthy' },
    { name: 'stale', state: { ...baseState, stale: true }, expected: 'stale' },
    { name: 'unknown', state: undefined, expected: 'unknown' },
    { name: 'down without an observed container', state: { ...baseState, actual_state: null, health: 'down' as const }, expected: 'down' },
    {
      name: 'degraded',
      state: { ...baseState, actual_state: { ...baseState.actual_state!, status: 'exited' } },
      expected: 'degraded',
    },
  ])('shows $name from actual-state evidence', ({ state, expected }) => {
    render(<FixtureNode state={state} />)
    expect(screen.getByText(expected)).toBeInTheDocument()
  })
})
