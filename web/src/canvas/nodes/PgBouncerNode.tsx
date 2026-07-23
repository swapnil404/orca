import type { NodeProps } from '@xyflow/react'
import { NodeCard } from './NodeCard'
import type { InfrastructureNode } from './types'

export function PgBouncerNode({ data }: NodeProps<InfrastructureNode>) {
  return <NodeCard {...data} accent="text-violet-300" />
}
