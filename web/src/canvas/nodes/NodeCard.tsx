import { Handle, Position } from '@xyflow/react'
import { RotateCw, TriangleAlert } from 'lucide-react'
import type { RestartPhase } from '../../lib/restart'
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
  restartPhase?: RestartPhase
  children?: ReactNode
}

export function NodeCard({ label, eyebrow, detail, status, accent, lifecycle, lifecycleLabel, restartPhase, children }: NodeCardProps) {
  const lifecycleClass = restartPhase ? (restartPhase === 'failed' ? 'border-[var(--critical)] shadow-[0_0_24px_rgba(244,63,94,0.12)]' : 'border-[var(--warning)] shadow-[0_0_24px_rgba(234,179,8,0.12)]') : lifecycle === 'error' ? 'border-[var(--critical)] opacity-100' : lifecycle === 'pending' ? 'border-[var(--border)] opacity-60' : 'border-[var(--border)]'
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
      {restartPhase && <div className={`mt-3 flex items-center gap-2 rounded-md border px-2.5 py-2 text-[10px] ${restartPhase === 'failed' ? 'border-[var(--critical)]/30 bg-[var(--critical)]/10 text-[var(--critical)]' : 'border-[var(--warning)]/30 bg-[var(--warning)]/10 text-[var(--warning)]'}`}>{restartPhase === 'failed' ? <TriangleAlert className="h-3.5 w-3.5" /> : <RotateCw className="h-3.5 w-3.5 motion-safe:animate-spin" />}{restartPhase === 'failed' ? 'Restart failed · see progress' : restartPhase === 'recovering' ? 'Restarted · checking health' : 'Restart pending · awaiting agent'}</div>}
      {children}
      <Handle type="source" position={Position.Right} className="!h-2 !w-2 !border-2 !border-[#0c0c0d] !bg-[var(--accent)]" />
    </motion.article>
  )
}
