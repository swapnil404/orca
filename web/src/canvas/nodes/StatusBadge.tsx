import type { NodeStatus } from '../status'
import { statusVisuals } from '../statusVisuals'

interface StatusBadgeProps {
  status: NodeStatus
}

export function StatusBadge({ status }: StatusBadgeProps) {
  const visual = statusVisuals[status]
  return (
    <span className={`rounded-full border px-2 py-0.5 font-mono text-[9px] font-medium ${visual.className}`}>
      {visual.label}
    </span>
  )
}
