import { Link, createFileRoute } from '@tanstack/react-router'
import { ArrowLeft, Blocks, RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { useEffect } from 'react'
import { ApiError, getProjectTopology, updateCluster } from '../../../api'
import { isReportStale } from '../../../canvas/status'
import { useProjectEvents } from '../../../hooks/useProjectEvents'
import { useTopologyStore } from '../../../store/topology'
import type { Cluster, ClusterInput, ProjectClusterState } from '../../../types/resources'

export const Route = createFileRoute('/_authenticated/projects/$projectId_/extensions')({
  ssr: false,
  loader: ({ params }) => getProjectTopology(params.projectId),
  component: ProjectExtensionsPage,
})

const actionClass = 'inline-flex min-w-20 items-center justify-center rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--panel)] px-3 py-2 text-xs font-medium text-[var(--text-2)] hover:border-[var(--text-3)] hover:text-[var(--text)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)] disabled:cursor-not-allowed disabled:opacity-50'

function clusterInput(cluster: Cluster, extensions: string[]): ClusterInput {
  return {
    name: cluster.name,
    postgres_version: cluster.postgres_version,
    parameters: cluster.parameters,
    replica_count: cluster.replica_count,
    enabled_extensions: extensions,
    pgbouncer_enabled: cluster.pgbouncer_enabled,
    pg_bouncer: cluster.pg_bouncer,
    pg_back_rest: cluster.pg_back_rest,
  }
}

function ProjectExtensionsPage() {
  const initialTopology = Route.useLoaderData()
  const { projectId } = Route.useParams()
  const [clusters, setClusters] = useState(initialTopology.clusters)
  const [pending, setPending] = useState('')
  const [message, setMessage] = useState('')
  const snapshot = useTopologyStore((state) => state.snapshot)
  useProjectEvents(projectId)

  const states = snapshot?.project_id === projectId ? snapshot.clusters : []
  const [now, setNow] = useState(() => Date.now())
  const fresh = states.length > 0 && states.every((state) => state.last_seen !== undefined && !isReportStale(state, now))
  const extensionNames = collectExtensionNames(clusters, states)
  useEffect(() => {
    if (snapshot?.project_id === projectId && snapshot.desired_clusters) setClusters(snapshot.desired_clusters)
  }, [projectId, snapshot?.desired_clusters, snapshot?.project_id])
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 15_000)
    return () => window.clearInterval(timer)
  }, [])

  async function changeDesired(cluster: Cluster, extension: string, enabled: boolean) {
    const operation = `${cluster.id}:${extension}`
    const next = enabled
      ? [...new Set([...cluster.enabled_extensions, extension])].sort()
      : cluster.enabled_extensions.filter((item) => item !== extension)
    setPending(operation)
    setMessage('')
    try {
      const updated = await updateCluster(cluster.id, clusterInput(cluster, next))
      setClusters((current) => current.map((item) => item.id === updated.id ? updated : item))
      setMessage(`${extension} ${enabled ? 'installation' : 'removal'} saved to desired state for ${cluster.name}.`)
    } catch (cause) {
      setMessage(cause instanceof ApiError ? cause.message : `Could not update ${extension} on ${cluster.name}.`)
    } finally {
      setPending('')
    }
  }

  async function retry(cluster: Cluster, extension: string) {
    const operation = `${cluster.id}:${extension}`
    setPending(operation)
    setMessage('')
    try {
      const updated = await updateCluster(cluster.id, clusterInput(cluster, cluster.enabled_extensions))
      setClusters((current) => current.map((item) => item.id === updated.id ? updated : item))
      setMessage(`Desired state re-sent to the agent for ${cluster.name}.`)
    } catch (cause) {
      setMessage(cause instanceof ApiError ? cause.message : `Could not retry reconciliation for ${cluster.name}.`)
    } finally {
      setPending('')
    }
  }

  return (
    <main className="min-h-screen px-4 py-8 text-[var(--text)] sm:px-8 lg:px-12 lg:py-10">
      <div className="mx-auto max-w-[1440px]">
        <header className="mb-8 border-b border-[var(--border)] pb-7">
          <Link to="/projects/$projectId" params={{ projectId }} search={{ tab: 'topology', restore: undefined }} className="mb-5 inline-flex items-center gap-2 text-xs text-[var(--text-3)] hover:text-[var(--text)]"><ArrowLeft className="h-3.5 w-3.5" />Topology</Link>
          <div className="flex flex-wrap items-end justify-between gap-5">
            <div><p className="mb-2 font-mono text-[10px] uppercase tracking-[0.18em] text-[var(--text-3)]">{initialTopology.project.name} / PostgreSQL</p><h1 className="text-2xl font-semibold tracking-[-0.03em] sm:text-3xl">Project extensions</h1><p className="mt-2 max-w-2xl text-sm leading-6 text-[var(--text-2)]">Installed state and desired-state reconciliation across every primary node in this project.</p></div>
            <span className="inline-flex items-center gap-2 rounded-full border border-[var(--border)] bg-[var(--panel)] px-3 py-2 text-xs text-[var(--text-2)]"><span className={`h-1.5 w-1.5 rounded-full ${fresh ? 'bg-[var(--healthy)]' : 'bg-[var(--text-3)]'}`} />{fresh ? 'Live agent data' : 'Agent data stale or unavailable'}</span>
          </div>
        </header>

        <section className="mb-6 border border-[var(--border)] bg-[var(--panel)] px-4 py-3 text-xs leading-5 text-[var(--text-2)] sm:px-5">
          <strong className="text-[var(--text)]">Versions are observed, not pinned.</strong> Desired state currently records extension presence only. Upgrade is disabled because no version target or upgrade operation exists in the API/reconciler contract; actual versions are never labeled up to date without that comparison.
        </section>

        <p role="status" className="mb-4 min-h-5 text-xs text-[var(--text-3)]">{message}</p>
        {extensionNames.length === 0 ? <EmptyState /> : <div className="space-y-5">{extensionNames.map((extension) => <ExtensionSection key={extension} extension={extension} clusters={clusters} states={states} pending={pending} onChange={changeDesired} onRetry={retry} />)}</div>}
      </div>
    </main>
  )
}

interface ExtensionSectionProps {
  extension: string
  clusters: Cluster[]
  states: ProjectClusterState[]
  pending: string
  onChange: (cluster: Cluster, extension: string, enabled: boolean) => Promise<void>
  onRetry: (cluster: Cluster, extension: string) => Promise<void>
}

function ExtensionSection({ extension, clusters, states, pending, onChange, onRetry }: ExtensionSectionProps) {
  const installedNodes = clusters.filter((cluster) => states.find((state) => state.cluster_id === cluster.id)?.actual_state?.enabled_extensions?.includes(extension))
  const method = states.map((state) => state.actual_state?.extension_update_methods?.[extension]).find((value) => value !== undefined)

  return (
    <section className="overflow-hidden rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--card)]">
      <div className="flex flex-wrap items-start justify-between gap-4 border-b border-[var(--border)] px-5 py-4 sm:px-6">
        <div className="flex items-start gap-3"><span className="grid h-9 w-9 place-items-center rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--panel)]"><Blocks className="h-4 w-4 text-[var(--accent)]" /></span><div><h2 className="font-mono text-sm font-semibold">{extension}</h2><p className="mt-1 text-xs text-[var(--text-3)]">{installedNodes.length ? `Installed on ${installedNodes.map((cluster) => cluster.name).join(', ')}` : 'Not currently reported as installed'}</p></div></div>
        <MethodBadge method={method} />
      </div>
      <div className="overflow-x-auto">
        <table className="w-full min-w-[820px] border-collapse text-left text-sm">
          <thead className="bg-[var(--panel)] font-mono text-[10px] uppercase tracking-[0.12em] text-[var(--text-3)]"><tr><th className="border-b border-[var(--border)] px-6 py-3 font-medium">Node</th><th className="border-b border-[var(--border)] px-6 py-3 font-medium">Desired</th><th className="border-b border-[var(--border)] px-6 py-3 font-medium">Actual version</th><th className="border-b border-[var(--border)] px-6 py-3 font-medium">Reconciliation</th><th className="border-b border-[var(--border)] px-6 py-3 text-right font-medium">Actions</th></tr></thead>
          <tbody className="divide-y divide-[var(--border-soft)]">{clusters.map((cluster) => {
            const actual = states.find((state) => state.cluster_id === cluster.id)?.actual_state
            const desired = cluster.enabled_extensions.includes(extension)
            const installed = actual?.enabled_extensions?.includes(extension) ?? false
            const reported = actual?.enabled_extensions !== undefined
            const drift = reported && desired !== installed
            const busy = pending === `${cluster.id}:${extension}`
            return <tr key={cluster.id}><td className="px-6 py-4"><p className="font-medium">{cluster.name}</p><p className="mt-0.5 font-mono text-[10px] text-[var(--text-3)]">primary · {cluster.host_id}</p></td><td className="px-6 py-4 text-xs">{desired ? 'Installed' : 'Absent'}<p className="mt-0.5 text-[var(--text-3)]">Version not pinned</p></td><td className="px-6 py-4 font-mono text-xs">{installed ? (actual?.extension_versions?.[extension] || 'Version not reported') : 'Not installed'}</td><td className="px-6 py-4"><DriftBadge reported={reported} drift={drift} desired={desired} /></td><td className="px-6 py-4"><div className="flex justify-end gap-2">{drift ? <button type="button" disabled={busy} onClick={() => void onRetry(cluster, extension)} className={actionClass}><RefreshCw className="mr-1.5 h-3.5 w-3.5" />Retry</button> : null}<button type="button" disabled={busy} onClick={() => void onChange(cluster, extension, !desired)} className={actionClass}>{busy ? 'Saving...' : desired ? 'Remove' : 'Install'}</button><button type="button" disabled title="No desired extension-version contract exists" className={actionClass}>Upgrade</button></div></td></tr>
          })}</tbody>
        </table>
      </div>
    </section>
  )
}

function collectExtensionNames(clusters: Cluster[], states: ProjectClusterState[]): string[] {
  const names = new Set(clusters.flatMap((cluster) => cluster.enabled_extensions))
  for (const state of states) {
    for (const extension of state.actual_state?.enabled_extensions ?? []) names.add(extension)
    for (const extension of Object.keys(state.actual_state?.extension_update_methods ?? {})) names.add(extension)
  }
  return [...names].sort()
}

function MethodBadge({ method }: { method: 'hot_apply' | 'restart' | undefined }) {
  const label = method === 'restart' ? 'Restart required' : method === 'hot_apply' ? 'Hot apply' : 'Method not reported'
  return <span className={`rounded-full border px-2.5 py-1 font-mono text-[10px] uppercase tracking-wide ${method === 'restart' ? 'border-[var(--warning)]/30 text-[var(--warning)]' : 'border-[var(--border)] text-[var(--text-3)]'}`}>{label}</span>
}

function DriftBadge({ reported, drift, desired }: { reported: boolean; drift: boolean; desired: boolean }) {
  if (!reported) return <span className="text-xs text-[var(--text-3)]">Actual state unavailable</span>
  if (!drift) return <span className="text-xs text-[var(--healthy)]">Presence converged</span>
  return <span className="text-xs text-[var(--warning)]">{desired ? 'Install pending or failed' : 'Removal pending or failed'}</span>
}

function EmptyState() {
  return <section className="rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--card)] px-6 py-16 text-center"><Blocks className="mx-auto mb-4 h-7 w-7 text-[var(--text-3)]" /><h2 className="font-medium">Extension capabilities not reported</h2><p className="mx-auto mt-2 max-w-xl text-sm text-[var(--text-2)]">Connect an updated agent to load its supported extension catalog. No client-side catalog or restart classification is guessed.</p></section>
}
