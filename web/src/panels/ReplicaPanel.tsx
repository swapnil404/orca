import type { ReplicaNodeData } from '../canvas/nodes/types'
import { PanelLayout, StateRow } from './PanelLayout'

interface ReplicaPanelProps {
  resource: ReplicaNodeData
  onClose: () => void
}

export function ReplicaPanel({ resource, onClose }: ReplicaPanelProps) {
  return (
    <PanelLayout title={`Replica ${resource.replicaID}`} eyebrow="Replica state" onClose={onClose}>
      <dl>
        <StateRow label="Desired" value="Present" />
        <StateRow label="Container" value={resource.actual?.status ?? 'Unknown'} />
        <StateRow label="Streaming" value={resource.actual?.streaming_state ?? 'Unknown'} />
        <StateRow label="Standby connected" value={resource.actual?.standby_connected === undefined ? 'Unknown' : resource.actual.standby_connected ? 'Yes' : 'No'} />
        <StateRow label="Replication lag" value={resource.actual?.replication_lag_bytes === undefined ? 'Unknown' : `${resource.actual.replication_lag_bytes} bytes`} />
        <StateRow label="Lag status" value={resource.actual?.replication_lag_status ?? 'Unknown'} />
      </dl>
    </PanelLayout>
  )
}
