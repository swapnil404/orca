import type { NodeProps } from '@xyflow/react'
import { NodeCard } from './NodeCard'
import type { InfrastructureNode } from './types'

export function ExtensionNode({ data }: NodeProps<InfrastructureNode>) {
  return <NodeCard {...data} accent="text-[var(--accent)]" lifecycle={data.kind === 'extension' && data.pendingInstall ? 'pending' : undefined} lifecycleLabel="pending install" />
}
