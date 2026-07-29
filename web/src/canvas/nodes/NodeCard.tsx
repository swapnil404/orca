import { Handle, Position } from '@xyflow/react'
import type { ReactNode } from 'react'
import type { NodeStatus } from '../status'
import { StatusBadge } from './StatusBadge'

interface NodeCardProps {
  label: string
  eyebrow: string
  detail: string
  status: NodeStatus
  accent: string
  children?: ReactNode
}

export function NodeCard({ label, eyebrow, detail, status, accent, children }: NodeCardProps) {
  return (
    <article className="group w-64 rounded-[var(--radius-xl)] border border-[var(--border)] bg-[var(--card)] p-4 shadow-[0_18px_50px_rgba(0,0,0,0.26)] transition duration-[var(--dur-base)] ease-[var(--ease-premium)] hover:-translate-y-0.5 hover:border-[var(--accent)] hover:shadow-[0_22px_60px_rgba(0,0,0,0.34)]">
      <Handle type="target" position={Position.Left} className="!h-2 !w-2 !border-2 !border-[#0c0c0d] !bg-[var(--accent)]" />
      <div className="mb-4 flex items-start justify-between gap-3">
        <div>
          <p className={`mb-1.5 font-mono text-[9px] font-semibold uppercase tracking-[0.18em] ${accent}`}>{eyebrow}</p>
          <h2 className="max-w-36 truncate text-[15px] font-semibold tracking-[-0.02em] text-[var(--text)]">{label}</h2>
        </div>
        <StatusBadge status={status} />
      </div>
      <p className="text-xs leading-5 text-[var(--text-2)]">{detail}</p>
      {children}
      <Handle type="source" position={Position.Right} className="!h-2 !w-2 !border-2 !border-[#0c0c0d] !bg-[var(--accent)]" />
    </article>
  )
}
