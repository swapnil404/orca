import { describe, expect, it } from 'vitest'
import type { ProjectStateSnapshot } from '../types/resources'
import { connectProjectEvents } from './project-events'

type Listener = (event: Event | MessageEvent<string>) => void

class FakeWebSocket {
  private readonly listeners = new Map<string, Listener[]>()

  addEventListener(type: string, listener: Listener): void {
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), listener])
  }

  close(): void {
    this.dispatch('close', new Event('close'))
  }

  sendSnapshot(snapshot: ProjectStateSnapshot): void {
    this.dispatch('message', new MessageEvent('message', { data: JSON.stringify(snapshot) }))
  }

  private dispatch(type: string, event: Event | MessageEvent<string>): void {
    this.listeners.get(type)?.forEach((listener) => listener(event))
  }
}

function snapshot(projectID: string, clusterIDs: string[]): ProjectStateSnapshot {
  return {
    type: 'project_state',
    project_id: projectID,
    clusters: clusterIDs.map((clusterID) => ({
      cluster_id: clusterID,
      host_id: 'host-1',
      actual_state: null,
      health: 'unknown',
      stale: false,
    })),
  }
}

describe('connectProjectEvents', () => {
  it('replaces the current snapshot instead of merging messages', () => {
    const socket = new FakeWebSocket()
    let current: ProjectStateSnapshot | null = null
    connectProjectEvents({
      projectID: 'project-1',
      socketFactory: () => socket as unknown as WebSocket,
      onSnapshot: (next) => {
        current = next
      },
    })

    socket.sendSnapshot(snapshot('project-1', ['old-cluster', 'retained-cluster']))
    socket.sendSnapshot(snapshot('project-1', ['new-cluster']))

    expect(current).toEqual(snapshot('project-1', ['new-cluster']))
  })
})
