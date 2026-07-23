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
    <div className="relative min-h-[560px] flex-1 overflow-hidden rounded-3xl border border-white/10 bg-[#0a1714]">
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
        <Background color="#27433b" gap={28} size={1} />
        <Controls showInteractive={false} className="!border-white/10 !bg-[#10201c] !fill-white" />
      </ReactFlow>
      <PanelHost selected={selected} onClose={() => setSelectedID(null)} />
    </div>
  )
}
