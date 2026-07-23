import type { PgBouncerNodeData } from '../canvas/nodes/types'
import { PanelLayout, StateRow } from './PanelLayout'

interface PgBouncerPanelProps {
  resource: PgBouncerNodeData
  onClose: () => void
}

export function PgBouncerPanel({ resource, onClose }: PgBouncerPanelProps) {
  return (
    <PanelLayout title="PgBouncer" eyebrow="Pool state" onClose={onClose}>
      <dl>
        <StateRow label="Desired" value={resource.cluster.pgbouncer_enabled ? 'Enabled' : 'Disabled'} />
        <StateRow label="Container" value={resource.actual?.status ?? 'Unknown'} />
        <StateRow label="Config reported" value={resource.actual?.config ? 'Yes' : 'No'} />
      </dl>
    </PanelLayout>
  )
}
