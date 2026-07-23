import type { NodeStatus } from '../status'

const statusStyles: Record<NodeStatus, string> = {
  healthy: 'border-emerald-400/40 bg-emerald-400/10 text-emerald-300',
  degraded: 'border-amber-400/40 bg-amber-400/10 text-amber-200',
  down: 'border-rose-400/40 bg-rose-400/10 text-rose-300',
  pending: 'border-sky-400/40 bg-sky-400/10 text-sky-300',
  stale: 'border-orange-400/40 bg-orange-400/10 text-orange-200',
  unknown: 'border-slate-400/30 bg-slate-400/10 text-slate-300',
}

interface StatusBadgeProps {
  status: NodeStatus
}

export function StatusBadge({ status }: StatusBadgeProps) {
  return (
    <span className={`rounded-full border px-2 py-0.5 text-[10px] font-semibold uppercase tracking-[0.16em] ${statusStyles[status]}`}>
      {status}
    </span>
  )
}
