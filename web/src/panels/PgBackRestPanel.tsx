import { Link } from '@tanstack/react-router'
import type { Cluster } from '../types/resources'
import { ContractUnavailable, PanelLayout } from './PanelLayout'

interface PgBackRestPanelProps {
  cluster: Cluster
  onClose: () => void
}

export function PgBackRestPanel({ cluster, onClose }: PgBackRestPanelProps) {
  return (
    <PanelLayout title={`${cluster.name} backups`} eyebrow="pgBackRest state" onClose={onClose}>
      <ContractUnavailable />
      <Link
        to="/projects/$projectId/backups"
        params={{ projectId: cluster.project_id }}
        className="mt-5 inline-flex text-sm font-medium text-[var(--accent)] hover:text-[var(--accent-hover)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]"
      >
        View full history →
      </Link>
    </PanelLayout>
  )
}
