import type { NodeProps } from '@xyflow/react'
import { NodeCard } from './NodeCard'
import type { InfrastructureNode } from './types'

export function PrimaryNode({ data }: NodeProps<InfrastructureNode>) {
  return (
    <NodeCard {...data} accent="text-emerald-300">
      <div className="mt-3 flex items-center justify-between border-t border-white/8 pt-3 text-[11px] text-slate-500">
        <span>Postgres {data.cluster.postgres_version}</span>
        <span>{data.state?.last_seen ? new Date(data.state.last_seen).toLocaleTimeString() : 'Never reported'}</span>
      </div>
    </NodeCard>
  )
}
