import type { NodeProps } from '@xyflow/react'
import { X } from 'lucide-react'
import { NodeCard } from './NodeCard'
import type { PendingInfrastructureNode } from './types'

const accents = {
  replica: 'text-[var(--streaming)]',
  pgbouncer: 'text-[#bd9cff]',
  pgbackrest: 'text-[var(--warning)]',
  extension: 'text-[var(--accent)]',
} as const

export function PendingNode({ data }: NodeProps<PendingInfrastructureNode>) {
  const isError = data.stage === 'error'
  return <NodeCard label={data.label} eyebrow={data.eyebrow} detail={data.error ?? data.detail} accent={isError ? 'text-[var(--critical)]' : accents[data.resourceType]} lifecycle={isError ? 'error' : 'pending'} lifecycleLabel={isError ? 'error' : data.stage === 'configuring' ? 'draft' : data.stage === 'submitting' ? 'saving' : 'awaiting agent'}>
    {isError ? <button type="button" className="nodrag mt-3 inline-flex items-center gap-1.5 text-[10px] font-medium text-[var(--critical)] hover:underline" onClick={(event) => { event.stopPropagation(); data.onDismiss() }}><X className="h-3 w-3" />Dismiss failed node</button> : null}
  </NodeCard>
}
