import { Background, Controls, ReactFlow, type NodeMouseHandler } from '@xyflow/react'
import { useEffect, useState } from 'react'
import type { Cluster, ProjectStateSnapshot } from '../types/resources'
import { PanelHost } from '../panels/PanelHost'
import { PgBouncerNode } from './nodes/PgBouncerNode'
import { PrimaryNode } from './nodes/PrimaryNode'
import { ReplicaNode } from './nodes/ReplicaNode'
import type { InfrastructureNode } from './nodes/types'
import { buildCanvasTopology } from './topology'

const nodeTypes = { primary: PrimaryNode, replica: ReplicaNode, pgbouncer: PgBouncerNode }

interface CanvasViewProps {
  clusters: Cluster[]
  snapshot: ProjectStateSnapshot | null
}

export function CanvasView({ clusters, snapshot }: CanvasViewProps) {
  const [selectedID, setSelectedID] = useState<string | null>(null)
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 15_000)
    return () => window.clearInterval(timer)
  }, [])
  const topology = buildCanvasTopology(clusters, snapshot, now)
  const selected = topology.nodes.find((node) => node.id === selectedID)?.data ?? null
  const selectNode: NodeMouseHandler<InfrastructureNode> = (_event, node) => setSelectedID(node.id)

  return (
    <div className="relative min-h-[560px] flex-1 overflow-hidden rounded-[var(--radius-xl)] border border-[var(--border)] bg-[#0c0c0d] shadow-[0_24px_80px_rgba(0,0,0,0.24),inset_0_1px_rgba(255,255,255,0.025)]">
      <div className="pointer-events-none absolute left-4 top-4 z-10 flex items-center gap-2 rounded-full bg-[var(--accent)] px-3 py-2 font-mono text-[9px] font-semibold uppercase tracking-[0.13em] text-[var(--accent-contrast)] sm:left-5 sm:top-5">
        <span className="h-1 w-1 rounded-full bg-[var(--accent-contrast)]" />
        Topology / {topology.nodes.length} nodes
      </div>
      <p className="pointer-events-none absolute right-5 top-5 z-10 hidden text-[11px] text-[var(--text-3)] sm:block">Select a node to inspect details</p>
      <ReactFlow
        nodes={topology.nodes}
        edges={topology.edges}
        nodeTypes={nodeTypes}
        onNodeClick={selectNode}
        onPaneClick={() => setSelectedID(null)}
        nodesDraggable={false}
        nodesConnectable={false}
        elementsSelectable
        fitView
        proOptions={{ hideAttribution: true }}
      >
        <Background color="#303034" gap={28} size={1} />
        <Controls showInteractive={false} className="!overflow-hidden !rounded-xl !border-[var(--border)] !bg-[var(--panel)] !fill-[var(--text-2)] !shadow-xl [&_button]:!border-[var(--border-soft)] [&_button]:!bg-transparent [&_button:hover]:!bg-white/5" />
      </ReactFlow>
      <PanelHost selected={selected} onClose={() => setSelectedID(null)} />
    </div>
  )
}
