import { Link } from '@tanstack/react-router'
import type { Cluster } from '../types/resources'
import { PanelLayout } from './PanelLayout'

interface ExtensionsPanelProps {
  cluster: Cluster
  onClose: () => void
}

export function ExtensionsPanel({ cluster, onClose }: ExtensionsPanelProps) {
  return <PanelLayout title={`${cluster.name} extensions`} eyebrow="Extension state" onClose={onClose}><p className="text-sm leading-6 text-[var(--text-2)]">Inspect observed versions, reconciliation drift, and extension actions across every node in this project.</p><Link to="/projects/$projectId/extensions" params={{ projectId: cluster.project_id }} onClick={onClose} className="mt-5 inline-flex w-full items-center justify-center rounded-[var(--radius-md)] bg-[var(--accent)] px-4 py-2.5 text-sm font-semibold text-[var(--accent-contrast)] hover:bg-[var(--accent-hover)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]">Open project extensions</Link></PanelLayout>
}
