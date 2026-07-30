import { Link } from '@tanstack/react-router'
import type { BackupJob, BackupStatus } from '../../types/resources'

interface BackupTableProps {
  jobs: BackupJob[]
  showProject: boolean
}

const statusLabels: Record<BackupStatus, string> = {
  succeeded: 'Succeeded',
  failed: 'Failed',
  pending: 'Pending',
  unknown: 'Unknown',
  not_configured: 'Not configured',
}

const statusTones: Record<BackupStatus, string> = {
  succeeded: 'border-[color-mix(in_srgb,var(--healthy)_35%,transparent)] bg-[color-mix(in_srgb,var(--healthy)_10%,transparent)] text-[var(--healthy)]',
  failed: 'border-[color-mix(in_srgb,var(--critical)_35%,transparent)] bg-[color-mix(in_srgb,var(--critical)_10%,transparent)] text-[var(--critical)]',
  pending: 'border-[var(--border)] bg-[var(--panel)] text-[var(--text-2)]',
  unknown: 'border-[var(--border)] bg-[var(--panel)] text-[var(--text-3)]',
  not_configured: 'border-[var(--border)] bg-transparent text-[var(--text-3)]',
}

function formatSize(size: number | null): string {
  if (size === null) return '—'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let value = size
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit += 1
  }
  return `${value.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`
}

export function BackupTable({ jobs, showProject }: BackupTableProps) {
  if (jobs.length === 0) {
    return <div className="rounded-[var(--radius-sm)] border border-dashed border-[var(--border)] bg-[var(--card)] px-5 py-10 text-center text-sm text-[var(--text-2)]">No backup jobs match these filters.</div>
  }

  return (
    <div className="overflow-x-auto rounded-[var(--radius-sm)] border border-[var(--border)]">
      <table className="w-full min-w-[780px] border-collapse text-left text-sm">
        <thead className="bg-[var(--panel)] text-xs font-medium text-[var(--text-2)]">
          <tr><th className="px-4 py-3">{showProject ? 'Project' : 'Cluster'}</th><th className="px-4 py-3">Last backup</th><th className="px-4 py-3 text-right">Size</th><th className="px-4 py-3">PITR</th><th className="px-4 py-3">Status</th><th className="px-4 py-3"><span className="sr-only">Action</span></th></tr>
        </thead>
        <tbody className="divide-y divide-[var(--border-soft)] bg-[var(--card)]">
          {jobs.map((job) => (
            <tr key={job.cluster_id} className="hover:bg-[var(--card-raised)]">
              <td className="px-4 py-3">
                {showProject ? <Link to="/projects/$projectId/backups" params={{ projectId: job.project_id }} search={{}} className="font-medium text-[var(--text)] hover:underline">{job.project_name}</Link> : <span className="font-medium text-[var(--text)]">{job.cluster_name}</span>}
                <div className="mt-0.5 font-mono text-[11px] text-[var(--text-3)]">{showProject ? job.cluster_name : job.cluster_id}</div>
              </td>
              <td className="px-4 py-3 font-mono text-xs text-[var(--text-2)]">{job.last_backup ? new Date(job.last_backup).toLocaleString() : 'Never'}</td>
              <td className="px-4 py-3 text-right font-mono text-xs">{formatSize(job.size_bytes)}</td>
              <td className="px-4 py-3 font-medium">{job.pitr_enabled ? 'Yes' : 'No'}</td>
              <td className="px-4 py-3"><span className={`inline-flex rounded-full border px-2 py-1 text-[11px] font-medium ${statusTones[job.status]}`}>{statusLabels[job.status]}</span></td>
              <td className="px-4 py-3 text-right">{job.pitr_enabled && job.last_backup ? <Link to="/projects/$projectId/backups" params={{ projectId: job.project_id }} search={{ restore: job.cluster_id }} className="text-xs font-medium text-[var(--accent)] hover:underline">Restore</Link> : <span className="text-xs text-[var(--text-3)]">Unavailable</span>}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
