import { Handle, Position } from '@xyflow/react'
import { motion } from 'framer-motion'
import type { ReactNode } from 'react'
import { fadeUp } from '../../lib/motion'
import type { NodeStatus } from '../status'
import { StatusBadge } from './StatusBadge'

interface NodeCardProps {
  label: string
  eyebrow: string
  detail: string
  status?: NodeStatus
  accent: string
  lifecycle?: 'pending' | 'error'
  lifecycleLabel?: string
  children?: ReactNode
}

export function NodeCard({ label, eyebrow, detail, status, accent, lifecycle, lifecycleLabel, children }: NodeCardProps) {
  const lifecycleClass = lifecycle === 'error' ? 'border-[var(--critical)] opacity-100' : lifecycle === 'pending' ? 'border-[var(--border)] opacity-60' : 'border-[var(--border)]'
  return (
    <motion.article variants={fadeUp} initial="hidden" animate="show" className={`group w-64 rounded-[var(--radius-lg)] border bg-[var(--card)] p-4 shadow-[0_18px_50px_rgba(0,0,0,0.26)] transition-colors hover:border-[var(--accent)] ${lifecycle ? `border-dashed ${lifecycleClass}` : lifecycleClass}`}>
      <Handle type="target" position={Position.Left} className="!h-2 !w-2 !border-2 !border-[#0c0c0d] !bg-[var(--accent)]" />
      <div className="mb-4 flex items-start justify-between gap-3">
        <div>
          <p className={`mb-1 text-xs ${accent}`}>{eyebrow}</p>
          <h2 className="max-w-36 truncate text-[15px] font-semibold tracking-[-0.02em] text-[var(--text)]">{label}</h2>
        </div>
        {lifecycle ? <span className={`rounded-full border px-2 py-0.5 font-mono text-[9px] font-medium ${lifecycle === 'error' ? 'border-rose-400/40 bg-rose-400/10 text-rose-300' : 'border-sky-400/40 bg-sky-400/10 text-sky-300'}`}>{lifecycleLabel ?? lifecycle}</span> : status ? <StatusBadge status={status} /> : null}
      </div>
      <p className="text-xs leading-5 text-[var(--text-2)]">{detail}</p>
      {children}
      <Handle type="source" position={Position.Right} className="!h-2 !w-2 !border-2 !border-[#0c0c0d] !bg-[var(--accent)]" />
    </motion.article>
  )
}
