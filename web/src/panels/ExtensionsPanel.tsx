import { Link } from '@tanstack/react-router'
import type { Cluster } from '../types/resources'
import type { ExtensionNodeData } from '../canvas/nodes/types'
import { PanelLayout } from './PanelLayout'

interface ExtensionsPanelProps {
  cluster: Cluster
  extension?: ExtensionNodeData
  onClose: () => void
}

export function ExtensionsPanel({ cluster, extension, onClose }: ExtensionsPanelProps) {
  const title = extension ? extension.extension : `${cluster.name} extensions`
  const description = extension
    ? `${extension.installed ? `Installed${extension.version ? ` at version ${extension.version}` : ''}` : 'Not installed'} on ${cluster.name}.`
    : 'Inspect observed versions, reconciliation drift, and extension actions across every node in this project.'
  return <PanelLayout title={title} eyebrow={extension ? `${cluster.name} extension` : 'Extension state'} onClose={onClose}><p className="text-sm leading-6 text-[var(--text-2)]">{description}</p><Link to="/projects/$projectId/extensions" params={{ projectId: cluster.project_id }} onClick={onClose} className="mt-5 inline-flex w-full items-center justify-center rounded-[var(--radius-md)] bg-[var(--accent)] px-4 py-2.5 text-sm font-semibold text-[var(--accent-contrast)] hover:bg-[var(--accent-hover)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]">Open project extensions</Link></PanelLayout>
}
