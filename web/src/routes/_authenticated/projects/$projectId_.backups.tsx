import { Link, createFileRoute } from '@tanstack/react-router'
import { Archive, ArrowLeft, CalendarClock, DatabaseBackup } from 'lucide-react'
import { useEffect, useState, type FormEvent } from 'react'
import { ApiError, getProjectTopology, listBackupJobs, listRestoreOperations, updateCluster } from '../../../api'
import { BackupTable } from '../../../components/backups/BackupTable'
import { RestoreWorkflow } from '../../../components/backups/RestoreWorkflow'
import { useProjectEvents } from '../../../hooks/useProjectEvents'
import { useTopologyStore } from '../../../store/topology'
import type { BackupJob, Cluster, PgBackRestConfig, RestoreOperation } from '../../../types/resources'

interface ProjectBackupsSearch {
  restore?: string
}

export const Route = createFileRoute('/_authenticated/projects/$projectId_/backups')({
  ssr: false,
  validateSearch: (search: Record<string, unknown>): ProjectBackupsSearch => ({
    restore: typeof search.restore === 'string' ? search.restore : undefined,
  }),
  loader: async ({ params }) => {
    const [topology, backupJobs, restoreOperations] = await Promise.all([
      getProjectTopology(params.projectId),
      listBackupJobs(params.projectId),
      listRestoreOperations(params.projectId),
    ])
    return { ...topology, backupJobs, restoreOperations }
  },
  component: ProjectBackupsPage,
})

const fieldClass = 'mt-2 w-full rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--panel)] px-3 py-2.5 font-mono text-sm text-[var(--text)] outline-none hover:border-[var(--text-3)] focus:border-[var(--accent)] focus:ring-2 focus:ring-[var(--accent-soft)] disabled:cursor-not-allowed disabled:opacity-50'
const buttonClass = 'inline-flex items-center justify-center rounded-[var(--radius-md)] bg-[var(--accent)] px-4 py-2.5 text-sm font-semibold text-[var(--accent-contrast)] hover:bg-[var(--accent-hover)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:ring-offset-2 focus:ring-offset-[var(--card)] disabled:cursor-not-allowed disabled:opacity-50'

function ProjectBackupsPage() {
  const initialTopology = Route.useLoaderData()
  const { projectId } = Route.useParams()
  const search = Route.useSearch()
  const [clusters, setClusters] = useState(initialTopology.clusters)
  const [backupJobs, setBackupJobs] = useState(initialTopology.backupJobs)
  const [restoreOperations, setRestoreOperations] = useState(initialTopology.restoreOperations)
  const snapshot = useTopologyStore((state) => state.snapshot)
  useProjectEvents(projectId)
  const projectSnapshot = snapshot?.project_id === projectId ? snapshot : null
  const backupClusters = clusters.filter((cluster) => cluster.pg_back_rest)
  const restoreClusterID = backupClusters.some((cluster) => cluster.id === search.restore) ? search.restore : undefined
  const [selectedClusterID, setSelectedClusterID] = useState(restoreClusterID ?? backupClusters[0]?.id ?? '')
  const selectedCluster = clusters.find((cluster) => cluster.id === selectedClusterID)
  const selectedOperations = restoreOperations.filter((operation) => operation.source_cluster_id === selectedClusterID)
  const restoreConflict = selectedOperations.some((operation) => operation.status === 'pending' || operation.status === 'ready' || operation.status === 'running')

  useEffect(() => {
    if (projectSnapshot) {
      setClusters(projectSnapshot.desired_clusters)
      setRestoreOperations(projectSnapshot.restore_operations)
    }
  }, [projectSnapshot])

  useEffect(() => {
    if (!projectSnapshot) return
    let cancelled = false
    void listBackupJobs(projectId).then((jobs) => {
      if (!cancelled) setBackupJobs(jobs)
    }).catch(() => {})
    return () => {
      cancelled = true
    }
  }, [projectId, projectSnapshot])

  function replaceOperation(updated: RestoreOperation) {
    setRestoreOperations((current) => [updated, ...current.filter((operation) => operation.id !== updated.id)])
  }

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
            <BackupInventory cluster={selectedCluster} jobs={backupJobs} />
            <div className="grid gap-6 xl:grid-cols-[minmax(0,1.08fr)_minmax(420px,0.92fr)]">
              <ScheduleEditor cluster={selectedCluster} disabled={restoreConflict} onUpdated={(updated) => setClusters((current) => current.map((cluster) => cluster.id === updated.id ? updated : cluster))} />
              {selectedCluster && <RestoreWorkflow key={selectedClusterID} cluster={selectedCluster} operations={selectedOperations} onOperation={replaceOperation} />}
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

function ScheduleEditor({ cluster, disabled, onUpdated }: { cluster: Cluster | undefined; disabled: boolean; onUpdated: (cluster: Cluster) => void }) {
  const config = cluster?.pg_back_rest
  const [saving, setSaving] = useState(false)
  const [message, setMessage] = useState('')

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!cluster || !config || disabled) return
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
        pg_hba_rules: cluster.pg_hba_rules,
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
        <fieldset disabled={saving || disabled} className="space-y-5"><label className="block text-xs font-medium text-[var(--text-2)]">Repository path<input required name="repo_path" defaultValue={config.repo_path} className={fieldClass} /></label>
        <div className="grid gap-4 sm:grid-cols-2"><NumberField name="retention_full" label="Full backups retained" value={config.retention_full} min={1} /><NumberField name="retention_diff" label="Differential backups retained" value={config.retention_diff} min={1} /></div>
        <div className="grid gap-4 sm:grid-cols-3"><NumberField name="full_interval_seconds" label="Full interval (sec)" value={config.full_interval_seconds} /><NumberField name="diff_interval_seconds" label="Diff interval (sec)" value={config.diff_interval_seconds} /><NumberField name="incr_interval_seconds" label="Incremental interval (sec)" value={config.incr_interval_seconds} /></div></fieldset>
        {disabled && <p role="status" className="rounded-[var(--radius-md)] border border-[var(--warning)]/25 bg-[var(--warning)]/5 px-3 py-2 text-xs leading-5 text-[var(--warning)]">Schedule edits are locked while this cluster has an active restore operation.</p>}
        <div className="flex flex-wrap items-center justify-between gap-3 border-t border-[var(--border)] pt-5"><p role="status" className="text-xs text-[var(--text-3)]">{message || 'Zero disables that backup interval.'}</p><button type="submit" disabled={saving || disabled} className={buttonClass}>{saving ? 'Saving...' : 'Save schedule'}</button></div>
      </form> : <p className="text-sm text-[var(--text-3)]">Select a configured cluster to edit its schedule.</p>}
    </section>
  )
}

function NumberField({ name, label, value, min = 0 }: { name: string; label: string; value: number; min?: number }) {
  return <label className="block text-xs font-medium text-[var(--text-2)]">{label}<input required name={name} type="number" min={min} step={1} defaultValue={value} className={fieldClass} /></label>
}
