import type { Cluster } from '../types/resources'
import { ContractUnavailable, PanelLayout } from './PanelLayout'

interface PgBackRestPanelProps {
  cluster: Cluster
  onClose: () => void
}

export function PgBackRestPanel({ cluster, onClose }: PgBackRestPanelProps) {
  return <PanelLayout title={`${cluster.name} backups`} eyebrow="pgBackRest state" onClose={onClose}><ContractUnavailable /></PanelLayout>
}
