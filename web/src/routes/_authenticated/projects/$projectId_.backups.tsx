import { Link, createFileRoute } from '@tanstack/react-router'
import { Archive, ArrowLeft, CalendarClock, ChevronRight, DatabaseBackup, ShieldAlert } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { ApiError, getProjectTopology, listBackupJobs, updateCluster } from '../../../api'
import { BackupTable } from '../../../components/backups/BackupTable'
import type { BackupJob, Cluster, PgBackRestConfig } from '../../../types/resources'

interface ProjectBackupsSearch {
  restore?: string
}

export const Route = createFileRoute('/_authenticated/projects/$projectId_/backups')({
  ssr: false,
  validateSearch: (search: Record<string, unknown>): ProjectBackupsSearch => ({
    restore: typeof search.restore === 'string' ? search.restore : undefined,
  }),
  loader: async ({ params }) => {
    const [topology, backupJobs] = await Promise.all([
      getProjectTopology(params.projectId),
      listBackupJobs(params.projectId),
    ])
    return { ...topology, backupJobs }
  },
  component: ProjectBackupsPage,
})

const fieldClass = 'mt-2 w-full rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--panel)] px-3 py-2.5 font-mono text-sm text-[var(--text)] outline-none hover:border-[var(--text-3)] focus:border-[var(--accent)] focus:ring-2 focus:ring-[var(--accent-soft)] disabled:cursor-not-allowed disabled:opacity-50'
const buttonClass = 'inline-flex items-center justify-center rounded-[var(--radius-md)] bg-[var(--accent)] px-4 py-2.5 text-sm font-semibold text-[var(--accent-contrast)] hover:bg-[var(--accent-hover)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:ring-offset-2 focus:ring-offset-[var(--card)] disabled:cursor-not-allowed disabled:opacity-50'
const secondaryButtonClass = 'inline-flex items-center justify-center rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--panel)] px-4 py-2.5 text-sm font-medium text-[var(--text-2)] hover:border-[var(--text-3)] hover:text-[var(--text)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]'

type RestoreStep = 'timeline' | 'target' | 'confirm'

function ProjectBackupsPage() {
  const initialTopology = Route.useLoaderData()
  const { projectId } = Route.useParams()
  const search = Route.useSearch()
  const [clusters, setClusters] = useState(initialTopology.clusters)
  const backupClusters = clusters.filter((cluster) => cluster.pg_back_rest)
  const restoreClusterID = backupClusters.some((cluster) => cluster.id === search.restore) ? search.restore : undefined
  const [selectedClusterID, setSelectedClusterID] = useState(restoreClusterID ?? backupClusters[0]?.id ?? '')
  const selectedCluster = clusters.find((cluster) => cluster.id === selectedClusterID)

  return (
    <main className="min-h-screen px-4 py-8 text-[var(--text)] sm:px-8 lg:px-12 lg:py-10">
      <div className="mx-auto max-w-[1440px]">
        <header className="mb-8 border-b border-[var(--border)] pb-7">
          <Link to="/projects/$projectId" params={{ projectId }} className="mb-5 inline-flex items-center gap-2 text-xs text-[var(--text-3)] hover:text-[var(--text)]"><ArrowLeft className="h-3.5 w-3.5" />Topology</Link>
          <div className="flex flex-wrap items-end justify-between gap-5">
            <div><p className="mb-2 font-mono text-[10px] uppercase tracking-[0.18em] text-[var(--text-3)]">{initialTopology.project.name} / pgBackRest</p><h1 className="text-2xl font-semibold tracking-[-0.03em] sm:text-3xl">Backup operations</h1><p className="mt-2 max-w-2xl text-sm leading-6 text-[var(--text-2)]">Inspect recovery coverage, manage schedules, and prepare point-in-time restores.</p></div>
            <label className="min-w-56 text-xs font-medium text-[var(--text-2)]">Cluster<select value={selectedClusterID} onChange={(event) => setSelectedClusterID(event.target.value)} className={fieldClass}><option value="">Select a cluster</option>{backupClusters.map((cluster) => <option key={cluster.id} value={cluster.id}>{cluster.name}</option>)}</select></label>
          </div>
        </header>

        {backupClusters.length === 0 ? <EmptyConfiguration /> : (
          <div className="space-y-6">
            <BackupInventory cluster={selectedCluster} jobs={initialTopology.backupJobs} />
            <div className="grid gap-6 xl:grid-cols-[minmax(0,1.08fr)_minmax(420px,0.92fr)]">
              <ScheduleEditor cluster={selectedCluster} onUpdated={(updated) => setClusters((current) => current.map((cluster) => cluster.id === updated.id ? updated : cluster))} />
              <RestoreWizard key={selectedClusterID} clusters={backupClusters} initialClusterID={selectedClusterID} />
            </div>
          </div>
        )}
      </div>
    </main>
  )
}

function EmptyConfiguration() {
  return <section className="rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--card)] px-6 py-16 text-center"><Archive className="mx-auto mb-4 h-7 w-7 text-[var(--text-3)]" /><h2 className="font-medium">No pgBackRest repositories configured</h2><p className="mx-auto mt-2 max-w-lg text-sm text-[var(--text-2)]">Enable pgBackRest on a cluster before backup schedules or recovery information can be managed here.</p></section>
}

function BackupInventory({ cluster, jobs }: { cluster: Cluster | undefined; jobs: BackupJob[] }) {
  const selectedJobs = cluster ? jobs.filter((job) => job.cluster_id === cluster.id) : []
  return (
    <section className="overflow-hidden rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--card)]">
      <div className="flex flex-wrap items-start justify-between gap-4 border-b border-[var(--border)] px-5 py-4 sm:px-6"><div><div className="flex items-center gap-2"><DatabaseBackup className="h-4 w-4 text-[var(--accent)]" /><h2 className="font-medium">Latest backup state</h2></div><p className="mt-1.5 text-xs text-[var(--text-3)]">{cluster ? `${cluster.name} · ${cluster.pg_back_rest?.repo_path}` : 'Select a cluster'}</p></div><span className="rounded-full border border-[var(--border)] px-2.5 py-1 font-mono text-[10px] uppercase tracking-wide text-[var(--text-3)]">Latest report</span></div>
      <div className="p-4 sm:p-5"><BackupTable jobs={selectedJobs} showProject={false} /></div>
    </section>
  )
}

function ScheduleEditor({ cluster, onUpdated }: { cluster: Cluster | undefined; onUpdated: (cluster: Cluster) => void }) {
  const config = cluster?.pg_back_rest
  const [saving, setSaving] = useState(false)
  const [message, setMessage] = useState('')

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!cluster || !config) return
    const form = new FormData(event.currentTarget)
    const pgBackRest: PgBackRestConfig = {
      repo_path: String(form.get('repo_path') ?? '').trim(),
      retention_full: Number(form.get('retention_full')),
      retention_diff: Number(form.get('retention_diff')),
      full_interval_seconds: Number(form.get('full_interval_seconds')),
      diff_interval_seconds: Number(form.get('diff_interval_seconds')),
      incr_interval_seconds: Number(form.get('incr_interval_seconds')),
    }
    setSaving(true)
    setMessage('')
    try {
      const updated = await updateCluster(cluster.id, {
        name: cluster.name,
        postgres_version: cluster.postgres_version,
        parameters: cluster.parameters,
        replica_count: cluster.replica_count,
        enabled_extensions: cluster.enabled_extensions,
        pgbouncer_enabled: cluster.pgbouncer_enabled,
        pg_bouncer: cluster.pg_bouncer,
        pg_back_rest: pgBackRest,
      })
      onUpdated(updated)
      setMessage('Schedule saved and desired state queued for the agent.')
    } catch (cause) {
      setMessage(cause instanceof ApiError ? cause.message : 'Could not update the backup schedule.')
    } finally {
      setSaving(false)
    }
  }

  return (
    <section className="rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--card)] p-5 sm:p-6">
      <div className="mb-6 flex items-center gap-2"><CalendarClock className="h-4 w-4 text-[var(--accent)]" /><div><h2 className="font-medium">Schedule and retention</h2><p className="mt-1 text-xs text-[var(--text-3)]">Values map directly to the current pgBackRest desired-state contract.</p></div></div>
      {config && cluster ? <form key={`${cluster.id}:${cluster.updated_at}`} onSubmit={handleSubmit} className="space-y-5">
        <label className="block text-xs font-medium text-[var(--text-2)]">Repository path<input required name="repo_path" defaultValue={config.repo_path} className={fieldClass} /></label>
        <div className="grid gap-4 sm:grid-cols-2"><NumberField name="retention_full" label="Full backups retained" value={config.retention_full} min={1} /><NumberField name="retention_diff" label="Differential backups retained" value={config.retention_diff} min={1} /></div>
        <div className="grid gap-4 sm:grid-cols-3"><NumberField name="full_interval_seconds" label="Full interval (sec)" value={config.full_interval_seconds} /><NumberField name="diff_interval_seconds" label="Diff interval (sec)" value={config.diff_interval_seconds} /><NumberField name="incr_interval_seconds" label="Incremental interval (sec)" value={config.incr_interval_seconds} /></div>
        <div className="flex flex-wrap items-center justify-between gap-3 border-t border-[var(--border)] pt-5"><p role="status" className="text-xs text-[var(--text-3)]">{message || 'Zero disables that backup interval.'}</p><button type="submit" disabled={saving} className={buttonClass}>{saving ? 'Saving...' : 'Save schedule'}</button></div>
      </form> : <p className="text-sm text-[var(--text-3)]">Select a configured cluster to edit its schedule.</p>}
    </section>
  )
}

function NumberField({ name, label, value, min = 0 }: { name: string; label: string; value: number; min?: number }) {
  return <label className="block text-xs font-medium text-[var(--text-2)]">{label}<input required name={name} type="number" min={min} step={1} defaultValue={value} className={fieldClass} /></label>
}

function RestoreWizard({ clusters, initialClusterID }: { clusters: Cluster[]; initialClusterID: string }) {
  const [step, setStep] = useState<RestoreStep>('timeline')
  const [recoveryTime, setRecoveryTime] = useState('')
  const [targetClusterID, setTargetClusterID] = useState(initialClusterID)
  const source = clusters.find((cluster) => cluster.id === initialClusterID)
  const target = clusters.find((cluster) => cluster.id === targetClusterID)
  const maxDateTime = new Date(Date.now() - new Date().getTimezoneOffset() * 60_000).toISOString().slice(0, 16)

  return (
    <section className="rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--card)] p-5 sm:p-6">
      <div className="mb-6 flex items-start gap-3"><span className="grid h-8 w-8 shrink-0 place-items-center rounded-full border border-[var(--warning)]/30 bg-[var(--warning)]/10"><ShieldAlert className="h-4 w-4 text-[var(--warning)]" /></span><div><h2 className="font-medium">Point-in-time restore</h2><p className="mt-1 text-xs leading-5 text-[var(--text-3)]">Prepare and review a destructive recovery operation.</p></div></div>
      <ol className="mb-7 grid grid-cols-3 border-y border-[var(--border)] py-3 text-[10px] uppercase tracking-[0.12em] text-[var(--text-3)]">{(['timeline', 'target', 'confirm'] as RestoreStep[]).map((item, index) => <li key={item} className={step === item ? 'text-[var(--accent)]' : ''}>{index + 1}. {item}</li>)}</ol>
      {step === 'timeline' && <div><label className="block text-xs font-medium text-[var(--text-2)]">Recovery date and time<input required type="datetime-local" max={maxDateTime} value={recoveryTime} onChange={(event) => setRecoveryTime(event.target.value)} className={fieldClass} /></label><ContractWarning /><div className="mt-6 flex justify-end"><button type="button" disabled={!recoveryTime || !source} onClick={() => setStep('target')} className={buttonClass}>Choose target<ChevronRight className="ml-1.5 h-4 w-4" /></button></div></div>}
      {step === 'target' && <div><label className="block text-xs font-medium text-[var(--text-2)]">Restore target<select value={targetClusterID} onChange={(event) => setTargetClusterID(event.target.value)} className={fieldClass}>{clusters.map((cluster) => <option key={cluster.id} value={cluster.id}>{cluster.name} ({cluster.host_id})</option>)}</select></label><p className="mt-3 text-xs leading-5 text-[var(--text-3)]">The current API has no restore target contract. This selection is review-only and is never submitted.</p><div className="mt-6 flex justify-between gap-3"><button type="button" onClick={() => setStep('timeline')} className={secondaryButtonClass}>Back</button><button type="button" disabled={!target} onClick={() => setStep('confirm')} className={buttonClass}>Review restore<ChevronRight className="ml-1.5 h-4 w-4" /></button></div></div>}
      {step === 'confirm' && <div><div className="rounded-[var(--radius-md)] border border-[var(--warning)]/25 bg-[var(--panel)] p-4"><p className="text-xs font-semibold uppercase tracking-[0.12em] text-[var(--warning)]">Confirmation required</p><p className="mt-3 text-sm leading-6 text-[var(--text)]">Restore <strong>{source?.name}</strong> to <strong>{new Date(recoveryTime).toLocaleString()}</strong> on target <strong>{target?.name}</strong>. A real restore may stop PostgreSQL, replace target data, replay WAL, and make data after the recovery time unavailable.</p></div><p className="mt-4 rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--panel)] p-3 text-xs leading-5 text-[var(--text-2)]"><strong className="text-[var(--text)]">Restore not yet wired.</strong> No server-side PITR endpoint exists, so execution is disabled. Nothing has been changed.</p><div className="mt-6 flex justify-between gap-3"><button type="button" onClick={() => setStep('target')} className={secondaryButtonClass}>Back</button><button type="button" disabled className={buttonClass}>Execute restore</button></div></div>}
    </section>
  )
}

function ContractWarning() {
  return <p className="mt-3 text-xs leading-5 text-[var(--text-3)]">The API does not expose backup-set retention boundaries yet. The selected time cannot be validated against recoverable WAL and will not be submitted.</p>
}
