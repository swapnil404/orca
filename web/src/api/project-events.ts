import type { ProjectStateSnapshot } from '../types/resources'

export type ProjectSocketFactory = (url: string) => WebSocket

interface ProjectEventClientOptions {
  projectID: string
  onSnapshot: (snapshot: ProjectStateSnapshot) => void
  onConnectionChange?: (connected: boolean) => void
  socketFactory?: ProjectSocketFactory
}

export interface ProjectEventConnection {
  close: () => void
}

function projectEventsURL(projectID: string): string {
  const url = new URL(`/projects/${encodeURIComponent(projectID)}/events`, window.location.origin)
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  return url.toString()
}

function parseSnapshot(data: unknown): ProjectStateSnapshot | null {
  if (typeof data !== 'string') {
    return null
  }
  const value = JSON.parse(data) as unknown
  if (
    typeof value !== 'object' ||
    value === null ||
    !('type' in value) ||
    value.type !== 'project_state' ||
    !('project_id' in value) ||
    typeof value.project_id !== 'string' ||
    !('clusters' in value) ||
    !Array.isArray(value.clusters)
  ) {
    return null
  }
  return value as ProjectStateSnapshot
}

export function connectProjectEvents(options: ProjectEventClientOptions): ProjectEventConnection {
  const socketFactory = options.socketFactory ?? ((url) => new WebSocket(url))
  let socket: WebSocket | null = null
  let reconnectTimer: ReturnType<typeof setTimeout> | undefined
  let closed = false
  let reconnectDelay = 1_000

  const connect = () => {
    socket = socketFactory(projectEventsURL(options.projectID))
    socket.addEventListener('open', () => {
      reconnectDelay = 1_000
      options.onConnectionChange?.(true)
    })
    socket.addEventListener('close', () => {
      options.onConnectionChange?.(false)
      if (!closed) {
        reconnectTimer = setTimeout(connect, reconnectDelay)
        reconnectDelay = Math.min(reconnectDelay * 2, 30_000)
      }
    })
    socket.addEventListener('message', (event) => {
      try {
        const snapshot = parseSnapshot(event.data)
        if (snapshot) {
          options.onSnapshot(snapshot)
        }
      } catch {
        // Ignore malformed frames; only complete project snapshots update state.
      }
    })
  }

  connect()
  return {
    close: () => {
      closed = true
      clearTimeout(reconnectTimer)
      socket?.close()
    },
  }
}
