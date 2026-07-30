import { createFileRoute } from '@tanstack/react-router'
import { useState } from 'react'
import { listBackupJobs } from '../../api'
import { BackupTable } from '../../components/backups/BackupTable'
import type { BackupStatus } from '../../types/resources'

export const Route = createFileRoute('/_authenticated/backups')({
  ssr: false,
  loader: () => listBackupJobs(),
  component: BackupsPage,
})

function BackupsPage() {
  const jobs = Route.useLoaderData()
  const [projectID, setProjectID] = useState('all')
  const [status, setStatus] = useState<BackupStatus | 'all'>('all')
  const projects = Array.from(new Map(jobs.map((job) => [job.project_id, job.project_name])).entries())
  const filteredJobs = jobs.filter((job) => (projectID === 'all' || job.project_id === projectID) && (status === 'all' || job.status === status))

  return (
    <main className="min-h-[calc(100vh-56px)] px-4 py-6 text-[var(--text)] sm:px-6 lg:px-8">
      <div className="mx-auto max-w-7xl">
        <header><p className="font-mono text-[10px] font-medium uppercase tracking-[0.16em] text-[var(--text-3)]">Recovery operations</p><h1 className="mt-1.5 text-xl font-semibold">Backups</h1><p className="mt-1 text-sm text-[var(--text-2)]">Latest pgBackRest job state across projects you can access.</p></header>
        <div className="my-5 flex flex-wrap gap-3">
          <label className="text-xs font-medium text-[var(--text-2)]">Project<select value={projectID} onChange={(event) => setProjectID(event.target.value)} className="ml-2 rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--panel)] px-3 py-2 text-sm text-[var(--text)]"><option value="all">All projects</option>{projects.map(([id, name]) => <option key={id} value={id}>{name}</option>)}</select></label>
          <label className="text-xs font-medium text-[var(--text-2)]">Status<select value={status} onChange={(event) => setStatus(event.target.value as BackupStatus | 'all')} className="ml-2 rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--panel)] px-3 py-2 text-sm text-[var(--text)]"><option value="all">All statuses</option><option value="succeeded">Succeeded</option><option value="failed">Failed</option><option value="pending">Pending</option><option value="unknown">Unknown</option><option value="not_configured">Not configured</option></select></label>
          <span className="self-center font-mono text-xs text-[var(--text-3)]">{filteredJobs.length} of {jobs.length} jobs</span>
        </div>
        <BackupTable jobs={filteredJobs} showProject />
      </div>
    </main>
  )
}
