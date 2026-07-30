import type { NodeProps } from '@xyflow/react'
import { NodeCard } from './NodeCard'
import type { InfrastructureNode } from './types'

export function PgBackRestNode({ data }: NodeProps<InfrastructureNode>) {
  return <NodeCard {...data} accent="text-[var(--warning)]" />
}
