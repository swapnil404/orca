import type { InfrastructureNodeData } from '../canvas/nodes/types'
import { ClusterPanel } from './ClusterPanel'
import { PgBouncerPanel } from './PgBouncerPanel'
import { ReplicaPanel } from './ReplicaPanel'

interface PanelHostProps {
  selected: InfrastructureNodeData | null
  onClose: () => void
}

export function PanelHost({ selected, onClose }: PanelHostProps) {
  if (!selected) return null
  if (selected.kind === 'cluster') return <ClusterPanel resource={selected} onClose={onClose} />
  if (selected.kind === 'replica') return <ReplicaPanel resource={selected} onClose={onClose} />
  return <PgBouncerPanel resource={selected} onClose={onClose} />
}
