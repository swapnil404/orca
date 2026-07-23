import type { Cluster } from '../types/resources'
import { ContractUnavailable, PanelLayout } from './PanelLayout'

interface ExtensionsPanelProps {
  cluster: Cluster
  onClose: () => void
}

export function ExtensionsPanel({ cluster, onClose }: ExtensionsPanelProps) {
  return <PanelLayout title={`${cluster.name} extensions`} eyebrow="Extension state" onClose={onClose}><ContractUnavailable /></PanelLayout>
}
