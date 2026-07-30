import { Link } from '@tanstack/react-router'
import type { ActualBackup, Cluster } from '../types/resources'
import { PanelLayout, StateRow } from './PanelLayout'

interface PgBackRestPanelProps {
  cluster: Cluster
  actual?: ActualBackup
  onClose: () => void
}

export function PgBackRestPanel({ cluster, actual, onClose }: PgBackRestPanelProps) {
  return (
    <PanelLayout title={`${cluster.name} backups`} eyebrow="pgBackRest state" onClose={onClose}>
      <dl>
        <StateRow label="Repository" value={cluster.pg_back_rest?.repo_path ?? 'Not enabled'} />
        <StateRow label="Reported status" value={actual?.status ?? 'Unavailable'} />
        <StateRow label="Last success" value={actual?.last_success_unix_seconds ? new Date(actual.last_success_unix_seconds * 1000).toLocaleString() : 'Never reported'} />
        <StateRow label="Latest size" value={actual?.size_bytes === undefined ? 'Unavailable' : `${new Intl.NumberFormat().format(actual.size_bytes)} bytes`} />
      </dl>
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
