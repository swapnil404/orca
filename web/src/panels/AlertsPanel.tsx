import type { Cluster } from '../types/resources'
import { ContractUnavailable, PanelLayout } from './PanelLayout'

interface AlertsPanelProps {
  cluster: Cluster
  onClose: () => void
}

export function AlertsPanel({ cluster, onClose }: AlertsPanelProps) {
  return <PanelLayout title={`${cluster.name} alerts`} eyebrow="Alert state" onClose={onClose}><ContractUnavailable /></PanelLayout>
}
