import type { PrimaryNodeData } from '../canvas/nodes/types'
import { PanelLayout, StateRow } from './PanelLayout'

interface ClusterPanelProps {
  resource: PrimaryNodeData
  onClose: () => void
}

export function ClusterPanel({ resource, onClose }: ClusterPanelProps) {
  return (
    <PanelLayout title={resource.cluster.name} eyebrow="Cluster state" onClose={onClose}>
      <dl>
        <StateRow label="Desired version" value={resource.cluster.postgres_version} />
        <StateRow label="Actual version" value={resource.actual?.version ?? 'Unknown'} />
        <StateRow label="Container" value={resource.actual?.status ?? 'Unknown'} />
        <StateRow label="Health" value={resource.status} />
        <StateRow label="Host" value={resource.cluster.host_id} />
        <StateRow label="Replicas desired" value={resource.cluster.replica_count} />
        <StateRow label="Last report" value={resource.state?.last_seen ? new Date(resource.state.last_seen).toLocaleString() : 'Never'} />
      </dl>
    </PanelLayout>
  )
}
