import type { Cluster, ProjectStateSnapshot } from '../types/resources'
import type { LagTone, TopologyEdgeType } from './edges/TopologyEdge'
import { pgBouncerStatus, primaryStatus, replicaStatus } from './status'
import type { InfrastructureNode } from './nodes/types'

export interface CanvasTopology {
  nodes: InfrastructureNode[]
  edges: TopologyEdgeType[]
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

  clusters.forEach((cluster, clusterIndex) => {
    const state = snapshot?.clusters.find((candidate) => candidate.cluster_id === cluster.id)
    const primaryID = `cluster:${cluster.id}`
    const y = clusterIndex * 330
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
  })

  return { nodes, edges }
}
