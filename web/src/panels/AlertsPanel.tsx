import { Link } from '@tanstack/react-router'
import type { Cluster } from '../types/resources'
import { PanelLayout } from './PanelLayout'

interface AlertsPanelProps {
  cluster: Cluster
  onClose: () => void
}

export function AlertsPanel({ cluster, onClose }: AlertsPanelProps) {
  return <PanelLayout title={`${cluster.name} alerts`} eyebrow="Alert state" onClose={onClose}><p className="text-sm leading-6 text-[var(--text-2)]">Review active rules and the evaluator's firing and resolved history for this project.</p><Link to="/projects/$projectId/alerts" params={{ projectId: cluster.project_id }} search={{ tab: 'topology', restore: undefined }} className="mt-5 inline-flex rounded-[var(--radius-sm)] bg-[var(--accent)] px-3 py-2 text-sm font-medium text-[var(--accent-contrast)] hover:bg-[var(--accent-hover)]">Open project alerts</Link></PanelLayout>
}
