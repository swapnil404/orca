import type { Cluster, ProjectStateSnapshot } from '../types/resources'
import type { LagTone, TopologyEdgeType } from './edges/TopologyEdge'
import { pgBackRestStatus, pgBouncerStatus, primaryStatus, replicaStatus } from './status'
import type { InfrastructureNode } from './nodes/types'

export interface CanvasTopology {
  nodes: InfrastructureNode[]
  edges: TopologyEdgeType[]
}

export function extensionNodeID(clusterID: string, extension: string): string {
  return `extension:${clusterID}:${encodeURIComponent(extension)}`
}

function lagTone(status: string | undefined): LagTone {
  if (status === 'known') return 'healthy'
  if (status === 'lagging') return 'warning'
  if (status === 'critical') return 'critical'
  return 'unknown'
}

function formatLag(value: string | number | undefined): string | undefined {
  if (value === undefined) return undefined
  try {
    return `${new Intl.NumberFormat().format(BigInt(value))} B lag`
  } catch {
    return `${value} B lag`
  }
}

export function buildCanvasTopology(clusters: Cluster[], snapshot: ProjectStateSnapshot | null, now = Date.now()): CanvasTopology {
  const nodes: InfrastructureNode[] = []
  const edges: TopologyEdgeType[] = []

  let y = 0
  clusters.forEach((cluster) => {
    const state = snapshot?.clusters.find((candidate) => candidate.cluster_id === cluster.id)
    const primaryID = `cluster:${cluster.id}`
    nodes.push({
      id: primaryID,
      type: 'primary',
      position: { x: 40, y },
      draggable: false,
      connectable: false,
      data: {
        kind: 'cluster',
        label: cluster.name,
        eyebrow: 'Primary',
        detail: state?.actual_state?.container_id ? `Container ${state.actual_state.container_id.slice(0, 12)}` : 'Awaiting actual state',
        status: primaryStatus(state, now),
        cluster,
        state,
        actual: state?.actual_state ?? undefined,
      },
    })

    cluster.replicas.forEach(({ id: replicaID }, index) => {
      const actual = state?.actual_state?.replicas?.find((replica) => replica.id === replicaID)
      const nodeID = `replica:${cluster.id}:${replicaID}`
      nodes.push({
        id: nodeID,
        type: 'replica',
        position: { x: 390, y: y + index * 140 - Math.max(0, cluster.replica_count - 1) * 60 },
        draggable: false,
        connectable: false,
        data: {
          kind: 'replica',
          replicaID,
          label: `Replica ${replicaID}`,
          eyebrow: 'Streaming replica',
          detail: actual?.replication_lag_bytes !== undefined ? `${actual.replication_lag_bytes} bytes lag` : 'Replication state unavailable',
          status: replicaStatus(state, actual, now),
          cluster,
          state,
          actual,
        },
      })
      edges.push({
        id: `${primaryID}->${nodeID}`,
        type: 'topology',
        source: primaryID,
        target: nodeID,
        data: { lagLabel: formatLag(actual?.replication_lag_bytes), lagTone: lagTone(actual?.replication_lag_status) },
      })
    })

    if (cluster.pgbouncer_enabled) {
      const actual = state?.actual_state?.pg_bouncer
      const nodeID = `pgbouncer:${cluster.id}`
      nodes.push({
        id: nodeID,
        type: 'pgbouncer',
        position: { x: 740, y },
        draggable: false,
        connectable: false,
        data: {
          kind: 'pgbouncer',
          label: 'PgBouncer',
          eyebrow: 'Connection pool',
          detail: actual?.container_id ? `Container ${actual.container_id.slice(0, 12)}` : 'Awaiting actual state',
          status: pgBouncerStatus(state, actual, now),
          cluster,
          state,
          actual,
        },
      })
      edges.push({ id: `${primaryID}->${nodeID}`, type: 'topology', source: primaryID, target: nodeID })
    }

    if (cluster.pg_back_rest) {
      const actual = state?.actual_state?.backup
      const nodeID = `pgbackrest:${cluster.id}`
      nodes.push({
        id: nodeID,
        type: 'pgbackrest',
        position: { x: 740, y: y + (cluster.pgbouncer_enabled ? 140 : 0) },
        draggable: false,
        connectable: false,
        data: {
          kind: 'pgbackrest',
          label: 'pgBackRest',
          eyebrow: 'Backup repository',
          detail: actual?.status ? `Agent reports ${actual.status}` : 'Awaiting applied backup config',
          status: pgBackRestStatus(state, actual, now),
          cluster,
          state,
          actual,
        },
      })
      edges.push({ id: `${primaryID}->${nodeID}`, type: 'topology', source: primaryID, target: nodeID })
    }

    cluster.enabled_extensions.forEach((extension, index) => {
      const reportedExtensions = state?.actual_state?.enabled_extensions
      const installed = reportedExtensions?.includes(extension) ?? false
      const nodeID = extensionNodeID(cluster.id, extension)
      nodes.push({
        id: nodeID,
        type: 'extension',
        position: { x: 1090, y: y + index * 120 },
        draggable: false,
        connectable: false,
        data: {
          kind: 'extension',
          extension,
          label: extension,
          eyebrow: 'PostgreSQL extension',
          detail: installed ? `Version ${state?.actual_state?.extension_versions?.[extension] ?? 'not reported'}` : reportedExtensions ? 'Install pending or failed' : 'Awaiting installation report',
          status: installed ? primaryStatus(state, now) : 'unknown',
          cluster,
          state,
          version: state?.actual_state?.extension_versions?.[extension],
          installed,
          pendingInstall: !installed && reportedExtensions === undefined,
        },
      })
      edges.push({ id: `${primaryID}->${nodeID}`, type: 'topology', source: primaryID, target: nodeID })
    })
    y += Math.max(330, cluster.enabled_extensions.length * 120 + 140)
  })

  return { nodes, edges }
}
