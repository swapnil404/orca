import type { NodeProps } from '@xyflow/react'
import { NodeCard } from './NodeCard'
import type { PrimaryInfrastructureNode } from './types'

export function PrimaryNode({ data }: NodeProps<PrimaryInfrastructureNode>) {
  return (
    <NodeCard {...data} accent="text-[var(--accent)]">
      <div className="mt-3 flex items-center justify-between border-t border-[var(--border-soft)] pt-3 font-mono text-[10px] text-[var(--text-3)]">
        <span>Postgres {data.cluster.postgres_version}</span>
        <span>{data.state?.last_seen ? new Date(data.state.last_seen).toLocaleTimeString() : 'Never reported'}</span>
      </div>
    </NodeCard>
  )
}
