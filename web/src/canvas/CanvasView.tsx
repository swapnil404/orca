import { Background, Controls, ReactFlow, type NodeMouseHandler } from '@xyflow/react'
import { useEffect, useState } from 'react'
import type { Cluster, ProjectStateSnapshot } from '../types/resources'
import { PanelHost } from '../panels/PanelHost'
import type { DetailSelection } from '../panels/types'
import { ServiceCards, type ServiceKind } from './ServiceCards'
import { TopologyEdge } from './edges/TopologyEdge'
import { PgBouncerNode } from './nodes/PgBouncerNode'
import { PrimaryNode } from './nodes/PrimaryNode'
import { ReplicaNode } from './nodes/ReplicaNode'
import type { InfrastructureNode } from './nodes/types'
import { buildCanvasTopology } from './topology'

const nodeTypes = { primary: PrimaryNode, replica: ReplicaNode, pgbouncer: PgBouncerNode }
const edgeTypes = { topology: TopologyEdge }

interface CanvasViewProps {
  clusters: Cluster[]
  snapshot: ProjectStateSnapshot | null
}

type SelectionTarget = { kind: 'node'; id: string } | { kind: ServiceKind; clusterID: string }

export function CanvasView({ clusters, snapshot }: CanvasViewProps) {
  const [selection, setSelection] = useState<SelectionTarget | null>(null)
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 15_000)
    return () => window.clearInterval(timer)
  }, [])
  const topology = buildCanvasTopology(clusters, snapshot, now)
  let selected: DetailSelection | null = null
  if (selection?.kind === 'node') {
    selected = topology.nodes.find((node) => node.id === selection.id)?.data ?? null
  } else if (selection) {
    const cluster = clusters.find((candidate) => candidate.id === selection.clusterID)
    const state = snapshot?.clusters.find((candidate) => candidate.cluster_id === selection.clusterID)
    if (cluster && selection.kind === 'pgbouncer') {
      selected = topology.nodes.find((node) => node.id === `pgbouncer:${cluster.id}`)?.data ?? {
        kind: 'pgbouncer', label: 'PgBouncer', eyebrow: 'Connection pool', detail: 'Not enabled',
        status: 'unknown', cluster, state, actual: state?.actual_state?.pg_bouncer,
      }
    } else if (cluster && selection.kind !== 'pgbouncer') {
      selected = { kind: selection.kind, cluster, state }
    }
  }
  const selectNode: NodeMouseHandler<InfrastructureNode> = (_event, node) => setSelection({ kind: 'node', id: node.id })
  const selectService = (kind: ServiceKind, clusterID: string) => setSelection({ kind, clusterID })

  return (
    <div className="flex flex-1 flex-col gap-3">
      <section className="relative min-h-[520px] flex-1 overflow-hidden rounded-[var(--radius-lg)] border border-[var(--border)] bg-[#0c0c0d] shadow-[0_24px_80px_rgba(0,0,0,0.2)]">
        <div className="pointer-events-none absolute left-4 top-4 z-10 flex items-center gap-2 rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--panel)] px-3 py-2 font-mono text-[10px] text-[var(--text-2)] sm:left-5 sm:top-5">
          <span className="h-1 w-1 rounded-full bg-[var(--accent)]" />
          Topology · {topology.nodes.length} nodes
        </div>
        <p className="pointer-events-none absolute right-5 top-5 z-10 hidden text-[11px] text-[var(--text-3)] sm:block">Select a node to inspect details</p>
        <ReactFlow
          nodes={topology.nodes}
          edges={topology.edges}
          nodeTypes={nodeTypes}
          edgeTypes={edgeTypes}
          onNodeClick={selectNode}
          onPaneClick={() => setSelection(null)}
          nodesDraggable={false}
          nodesConnectable={false}
          elementsSelectable
          fitView
          proOptions={{ hideAttribution: true }}
        >
          <Background color="#303034" gap={24} size={1} />
          <Controls showInteractive={false} className="!overflow-hidden !rounded-[var(--radius-lg)] !border-[var(--border)] !bg-[var(--panel)] !fill-[var(--text-2)] !shadow-xl [&_button]:!border-[var(--border-soft)] [&_button]:!bg-transparent [&_button:hover]:!bg-white/5" />
        </ReactFlow>
      </section>
      <ServiceCards clusters={clusters} states={snapshot?.clusters ?? []} now={now} onSelect={selectService} />
      <PanelHost selected={selected} onClose={() => setSelection(null)} />
    </div>
  )
}
