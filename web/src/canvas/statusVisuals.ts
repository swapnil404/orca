import type { NodeStatus } from './status'

export type DisplayStatus = NodeStatus | 'disabled' | 'unavailable'

interface StatusVisual {
  label: string
  className: string
}

export const statusVisuals: Record<DisplayStatus, StatusVisual> = {
  healthy: { label: 'healthy', className: 'border-emerald-400/40 bg-emerald-400/10 text-emerald-300' },
  degraded: { label: 'degraded', className: 'border-amber-400/40 bg-amber-400/10 text-amber-200' },
  down: { label: 'down', className: 'border-rose-400/40 bg-rose-400/10 text-rose-300' },
  pending: { label: 'pending', className: 'border-sky-400/40 bg-sky-400/10 text-sky-300' },
  stale: { label: 'stale', className: 'border-orange-400/40 bg-orange-400/10 text-orange-200' },
  unknown: { label: 'unknown', className: 'border-slate-400/30 bg-slate-400/10 text-slate-300' },
  disabled: { label: 'disabled', className: 'border-slate-400/20 bg-slate-400/5 text-[var(--text-3)]' },
  unavailable: { label: 'unavailable', className: 'border-slate-400/20 bg-slate-400/5 text-[var(--text-3)]' },
}
