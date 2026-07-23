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
    <article className="w-64 rounded-2xl border border-white/10 bg-[#10201c]/95 p-4 shadow-2xl shadow-black/30 backdrop-blur">
      <Handle type="target" position={Position.Left} className="!border-0 !bg-emerald-300" />
      <div className="mb-3 flex items-start justify-between gap-3">
        <div>
          <p className={`mb-1 text-[10px] font-bold uppercase tracking-[0.2em] ${accent}`}>{eyebrow}</p>
          <h2 className="max-w-36 truncate text-base font-semibold text-white">{label}</h2>
        </div>
        <StatusBadge status={status} />
      </div>
      <p className="text-xs leading-5 text-slate-400">{detail}</p>
      {children}
      <Handle type="source" position={Position.Right} className="!border-0 !bg-emerald-300" />
    </article>
  )
}
