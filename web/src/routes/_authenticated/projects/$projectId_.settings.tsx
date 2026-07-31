import { Link, createFileRoute, useNavigate } from '@tanstack/react-router'
import { AlertTriangle, ArrowLeft, KeyRound, Network, ServerCog, ShieldCheck, Trash2 } from 'lucide-react'
import { useEffect, useState, type FormEvent, type ReactNode } from 'react'
import { ApiError, deleteProject, getProjectTopology, listProjectHosts, updatePgBouncer, updatePgHba } from '../../../api'
import { PgHbaRulesEditor } from '../../../components/PgHbaRulesEditor'
import { useProjectEvents } from '../../../hooks/useProjectEvents'
import { useTopologyStore } from '../../../store/topology'
import type { ActualCluster, ActualPgBouncer, Cluster, PgBouncerConfig, PgBouncerPoolMode, PgHbaRule, ProjectHost, ReconciliationResult } from '../../../types/resources'

export const Route = createFileRoute('/_authenticated/projects/$projectId_/settings')({
  ssr: false,
  loader: async ({ params }) => {
    const [topology, hosts] = await Promise.all([getProjectTopology(params.projectId), listProjectHosts(params.projectId)])
    return { ...topology, hosts }
  },
  component: ProjectSettingsPage,
})

const fieldClass = 'mt-2 w-full rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--panel)] px-3 py-2.5 font-mono text-sm text-[var(--text)] outline-none hover:border-[var(--text-3)] focus:border-[var(--accent)] focus:ring-2 focus:ring-[var(--accent-soft)] disabled:cursor-not-allowed disabled:opacity-50'
const primaryButtonClass = 'inline-flex items-center justify-center rounded-[var(--radius-md)] bg-[var(--accent)] px-4 py-2.5 text-sm font-semibold text-[var(--accent-contrast)] hover:bg-[var(--accent-hover)] disabled:cursor-not-allowed disabled:opacity-50'

function ProjectSettingsPage() {
  const initial = Route.useLoaderData()
  const { projectId } = Route.useParams()
  const [clusters, setClusters] = useState(initial.clusters)
  const [hosts, setHosts] = useState(initial.hosts)
  const [hostStatusUnknown, setHostStatusUnknown] = useState(false)
  const snapshot = useTopologyStore((state) => state.snapshot)
  useProjectEvents(projectId)

  useEffect(() => {
    if (snapshot?.project_id === projectId) setClusters(snapshot.desired_clusters)
  }, [projectId, snapshot?.desired_clusters, snapshot?.project_id])

  useEffect(() => {
    let active = true
    const refresh = () => void listProjectHosts(projectId).then((nextHosts) => {
      if (!active) return
      setHosts(nextHosts)
      setHostStatusUnknown(false)
    }).catch(() => {
      if (active) setHostStatusUnknown(true)
    })
    const timer = window.setInterval(refresh, 10_000)
    return () => { active = false; window.clearInterval(timer) }
  }, [projectId])

  const pools = clusters.filter((cluster) => cluster.pgbouncer_enabled)
  return (
    <main className="min-h-screen px-4 py-8 text-[var(--text)] sm:px-8 lg:px-12 lg:py-10">
      <div className="mx-auto max-w-6xl">
        <header className="mb-8 border-b border-[var(--border)] pb-7">
          <Link to="/projects/$projectId" params={{ projectId }} search={{ tab: 'topology', restore: undefined }} className="mb-5 inline-flex items-center gap-2 text-xs text-[var(--text-3)] hover:text-[var(--text)]"><ArrowLeft className="h-3.5 w-3.5" />Topology</Link>
          <p className="mb-2 font-mono text-[10px] uppercase tracking-[0.18em] text-[var(--text-3)]">{initial.project.name} / control plane</p>
          <h1 className="text-2xl font-semibold tracking-[-0.03em] sm:text-3xl">Project settings</h1>
          <p className="mt-2 max-w-2xl text-sm leading-6 text-[var(--text-2)]">Manage connection pools and inspect the agents responsible for this project.</p>
        </header>

        <div className="space-y-6">
          <SettingsSection icon={ShieldCheck} title="PostgreSQL access control" description="Rules are persisted in order, applied to the primary and replicas, and reloaded without restarting PostgreSQL.">
            <div className="divide-y divide-[var(--border-soft)]">{clusters.map((cluster) => { const state = snapshot?.clusters.find((item) => item.cluster_id === cluster.id); return <PgHbaEditor key={`${cluster.id}:${cluster.updated_at}`} cluster={cluster} actual={state?.actual_state ?? undefined} stale={state?.stale ?? true} results={state?.reconciliation_results ?? []} onUpdated={(updated) => setClusters((current) => current.map((item) => item.id === updated.id ? updated : item))} /> })}</div>
          </SettingsSection>

          <SettingsSection icon={Network} title="PgBouncer pools" description="Each save is persisted with the cluster's complete desired state, then reconciled by its agent.">
            {pools.length === 0 ? <EmptyText>No PgBouncer-enabled clusters are configured.</EmptyText> : <div className="divide-y divide-[var(--border-soft)]">{pools.map((cluster) => { const state = snapshot?.clusters.find((item) => item.cluster_id === cluster.id); return <PoolEditor key={cluster.id} cluster={cluster} actual={state?.actual_state?.pg_bouncer} stale={state?.stale ?? true} onUpdated={(updated) => setClusters((current) => current.map((item) => item.id === updated.id ? updated : item))} /> })}</div>}
          </SettingsSection>

          <SettingsSection icon={KeyRound} title="Project secrets" description="Secret material must never be returned to or rendered by this page.">
            <div className="rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--panel)] p-4"><p className="text-sm font-medium">Secrets API unavailable</p><p className="mt-1.5 text-xs leading-5 text-[var(--text-3)]">The server has no project-secret list, rotate, or delete contract. No placeholder names or secret values are shown.</p></div>
          </SettingsSection>

          <SettingsSection icon={ServerCog} title="Agents" description="Connection state is derived from the control plane's in-memory WebSocket hub and refreshes every ten seconds.">
            {hostStatusUnknown && <p role="status" className="mb-4 rounded-[var(--radius-md)] border border-[var(--warning)]/25 bg-[var(--warning)]/5 px-3 py-2 text-xs text-[var(--warning)]">Host status refresh failed. Current connection state is unknown.</p>}
            {hosts.length === 0 ? <EmptyText>No hosts are assigned to this project.</EmptyText> : <div className="divide-y divide-[var(--border-soft)]">{hosts.map((host) => <AgentRow key={host.id} host={host} statusUnknown={hostStatusUnknown} />)}</div>}
          </SettingsSection>

          <DangerZone projectID={projectId} projectName={initial.project.name} />
        </div>
      </div>
    </main>
  )
}

interface SettingsSectionProps {
  icon: typeof Network
  title: string
  description: string
  children: ReactNode
}

function SettingsSection({ icon: Icon, title, description, children }: SettingsSectionProps) {
  return <section className="overflow-hidden rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--card)]"><div className="flex gap-3 border-b border-[var(--border)] px-5 py-4 sm:px-6"><Icon className="mt-0.5 h-4 w-4 shrink-0 text-[var(--accent)]" /><div><h2 className="font-medium">{title}</h2><p className="mt-1 text-xs leading-5 text-[var(--text-3)]">{description}</p></div></div><div className="px-5 py-5 sm:px-6">{children}</div></section>
}

function EmptyText({ children }: { children: string }) {
  return <p className="py-4 text-center text-sm text-[var(--text-3)]">{children}</p>
}

interface PgHbaEditorProps {
  cluster: Cluster
  actual?: ActualCluster
  stale: boolean
  results: ReconciliationResult[]
  onUpdated: (cluster: Cluster) => void
}

function rulesEqual(desired: PgHbaRule[], applied?: PgHbaRule[]): boolean {
  return applied !== undefined && JSON.stringify(desired) === JSON.stringify(applied)
}

function PgHbaEditor({ cluster, actual, stale, results, onUpdated }: PgHbaEditorProps) {
  const [rules, setRules] = useState<PgHbaRule[]>(cluster.pg_hba_rules)
  const [saving, setSaving] = useState(false)
  const [message, setMessage] = useState('')
  const primaryApplied = actual?.pg_hba_observed === true && rulesEqual(cluster.pg_hba_rules, actual.pg_hba_rules)
  const actualReplicas = new Map((actual?.replicas ?? []).map((replica) => [replica.id, replica]))
  const replicasApplied = cluster.replicas.every((desiredReplica) => { const replica = actualReplicas.get(desiredReplica.id); return replica?.pg_hba_observed === true && rulesEqual(cluster.pg_hba_rules, replica.pg_hba_rules) })
  const expectedReplicationCIDRs = cluster.replicas.length > 0 ? actual?.network_cidrs ?? [] : []
  const replicationApplied = JSON.stringify(expectedReplicationCIDRs) === JSON.stringify(actual?.pg_hba_replication_cidrs ?? [])
  const applied = !stale && primaryApplied && replicasApplied && replicationApplied
  const failure = [...results].reverse().find((result) => result.action === 'update_pg_hba' && result.status === 'failed')

  async function save() {
    setSaving(true)
    setMessage('')
    try {
      const updated = await updatePgHba(cluster.id, rules)
      onUpdated(updated)
      setMessage('Desired rules saved. Waiting for the agent to report the applied file.')
    } catch (cause) {
      setMessage(cause instanceof ApiError ? cause.message : 'Could not save access rules.')
    } finally {
      setSaving(false)
    }
  }

  const appliedRules = actual?.pg_hba_observed ? actual.pg_hba_rules ?? [] : undefined
  return <div className="py-5 first:pt-0 last:pb-0"><div className="mb-4 flex flex-wrap items-center justify-between gap-3"><div><h3 className="text-sm font-medium">{cluster.name}</h3><p className="mt-1 font-mono text-[11px] text-[var(--text-3)]">{cluster.id}</p></div><span className={`inline-flex items-center gap-2 rounded-full border px-2.5 py-1 font-mono text-[10px] uppercase tracking-wide ${applied ? 'border-[var(--healthy)]/30 text-[var(--healthy)]' : failure ? 'border-[var(--critical)]/30 text-[var(--critical)]' : 'border-[var(--warning)]/30 text-[var(--warning)]'}`}><span className={`h-1.5 w-1.5 rounded-full ${applied ? 'bg-[var(--healthy)]' : failure ? 'bg-[var(--critical)]' : 'bg-[var(--warning)]'}`} />{applied ? 'Applied' : failure ? 'Apply failed' : 'Desired differs'}</span></div><PgHbaRulesEditor rules={rules} disabled={saving} onChange={setRules} /><div className="mt-4 flex flex-wrap items-start justify-between gap-3 border-t border-[var(--border-soft)] pt-4"><div className="max-w-2xl text-xs leading-5 text-[var(--text-3)]"><p role="status">{message || failure?.error || (applied ? `${cluster.pg_hba_rules.length} desired rules match every reported PostgreSQL node.` : stale ? 'Applied state is unavailable or stale.' : `Desired: ${cluster.pg_hba_rules.length} rules. Primary reported: ${appliedRules?.length ?? 'unavailable'}.`)}</p>{appliedRules && !rulesEqual(cluster.pg_hba_rules, appliedRules) && <details className="mt-2"><summary className="cursor-pointer text-[var(--text-2)]">Show primary applied rules</summary><pre className="mt-2 overflow-x-auto rounded-[var(--radius-sm)] bg-[var(--panel)] p-3 font-mono text-[11px]">{appliedRules.map((rule) => `${rule.type} ${rule.database} ${rule.user} ${rule.address} ${rule.method}`.replace('  ', ' ')).join('\n') || '(none)'}</pre></details>}</div><button type="button" disabled={saving} onClick={save} className={primaryButtonClass}>{saving ? 'Saving...' : 'Save access rules'}</button></div></div>
}

interface PoolEditorProps {
  cluster: Cluster
  actual?: ActualPgBouncer
  stale: boolean
  onUpdated: (cluster: Cluster) => void
}

function appliedPoolConfig(content?: string): Partial<PgBouncerConfig> {
  if (!content) return {}
  const values: Record<string, string> = {}
  for (const line of content.split('\n')) {
    const [key, ...rest] = line.split('=')
    if (key && rest.length > 0) values[key.trim()] = rest.join('=').trim()
  }
  const mode = values.pool_mode
  const maxConnections = Number(values.max_client_conn)
  return {
    pool_mode: mode === 'session' || mode === 'transaction' || mode === 'statement' ? mode : undefined,
    max_connections: Number.isInteger(maxConnections) ? maxConnections : undefined,
  }
}

function PoolEditor({ cluster, actual, stale, onUpdated }: PoolEditorProps) {
  const [saving, setSaving] = useState(false)
  const [message, setMessage] = useState('')
  const applied = appliedPoolConfig(actual?.config)
  const isApplied = !stale && actual?.status === 'running' && applied.pool_mode === cluster.pg_bouncer.pool_mode && applied.max_connections === cluster.pg_bouncer.max_connections

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = new FormData(event.currentTarget)
    const poolMode = String(form.get('pool_mode')) as PgBouncerPoolMode
    const maxConnections = Number(form.get('max_connections'))
    setSaving(true)
    setMessage('')
    try {
      const updated = await updatePgBouncer(cluster.id, { pool_mode: poolMode, max_connections: maxConnections })
      onUpdated(updated)
      setMessage('Desired state saved. Waiting for the agent to report the applied config.')
    } catch (error) {
      setMessage(error instanceof ApiError ? error.message : 'Could not save pool settings.')
    } finally {
      setSaving(false)
    }
  }

  return <form onSubmit={handleSubmit} className="py-5 first:pt-0 last:pb-0"><div className="mb-4 flex flex-wrap items-center justify-between gap-3"><div><h3 className="text-sm font-medium">{cluster.name}</h3><p className="mt-1 font-mono text-[11px] text-[var(--text-3)]">{cluster.id}</p></div><span className={`inline-flex items-center gap-2 rounded-full border px-2.5 py-1 font-mono text-[10px] uppercase tracking-wide ${isApplied ? 'border-[var(--healthy)]/30 text-[var(--healthy)]' : 'border-[var(--warning)]/30 text-[var(--warning)]'}`}><span className={`h-1.5 w-1.5 rounded-full ${isApplied ? 'bg-[var(--healthy)]' : 'bg-[var(--warning)]'}`} />{isApplied ? 'Applied' : 'Awaiting agent'}</span></div><div className="grid gap-4 sm:grid-cols-2"><label className="text-xs font-medium text-[var(--text-2)]">Pool mode<select name="pool_mode" defaultValue={cluster.pg_bouncer.pool_mode} className={fieldClass}><option value="session">Session</option><option value="transaction">Transaction</option><option value="statement">Statement</option></select></label><label className="text-xs font-medium text-[var(--text-2)]">Maximum client connections<input name="max_connections" type="number" min="1" step="1" required defaultValue={cluster.pg_bouncer.max_connections} className={fieldClass} /></label></div><div className="mt-4 flex flex-wrap items-center justify-between gap-3"><p role="status" className="text-xs text-[var(--text-3)]">{message || (actual?.config ? `Reported: ${applied.pool_mode ?? 'unknown'} / ${applied.max_connections ?? 'unknown'} connections` : 'No applied PgBouncer config has been reported.')}</p><button disabled={saving} className={primaryButtonClass}>{saving ? 'Saving...' : 'Save pool config'}</button></div></form>
}

function AgentRow({ host, statusUnknown }: { host: ProjectHost; statusUnknown: boolean }) {
  const labels = { online: 'Connected', offline: 'Disconnected', never_connected: 'Never connected' } as const
  const online = !statusUnknown && host.status === 'online'
  return <div className="grid gap-3 py-4 first:pt-0 last:pb-0 sm:grid-cols-[1fr_auto_auto] sm:items-center"><div><p className="font-mono text-xs text-[var(--text)]">{host.id}</p><p className="mt-1 text-[11px] text-[var(--text-3)]">Host identifier</p></div><div className="flex items-center gap-2 text-xs text-[var(--text-2)]"><span className={`h-1.5 w-1.5 rounded-full ${online ? 'bg-[var(--healthy)]' : statusUnknown ? 'bg-[var(--warning)]' : 'bg-[var(--text-3)]'}`} />{statusUnknown ? 'Status unknown' : labels[host.status]}</div><div className="text-xs text-[var(--text-3)]"><span className="mr-2">Agent version</span><span className="font-mono text-[var(--text-2)]">Not reported</span></div></div>
}

function DangerZone({ projectID, projectName }: { projectID: string; projectName: string }) {
  const navigate = useNavigate()
  const [confirmation, setConfirmation] = useState('')
  const [deleting, setDeleting] = useState(false)
  const [error, setError] = useState('')

  async function handleDelete() {
    if (confirmation !== projectName) return
    setDeleting(true)
    setError('')
    try {
      await deleteProject(projectID)
      await navigate({ to: '/' })
    } catch (cause) {
      setError(cause instanceof ApiError ? cause.message : 'Could not delete this project.')
      setDeleting(false)
    }
  }

  return <section className="overflow-hidden rounded-[var(--radius-lg)] border border-[var(--critical)]/35 bg-[var(--card)]"><div className="flex gap-3 border-b border-[var(--critical)]/25 bg-[var(--critical)]/5 px-5 py-4 sm:px-6"><AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-[var(--critical)]" /><div><h2 className="font-medium text-[var(--critical)]">Danger zone</h2><p className="mt-1 text-xs leading-5 text-[var(--text-2)]">Deleting a project removes its desired clusters and instructs connected agents to delete their managed resources.</p></div></div><div className="px-5 py-5 sm:px-6"><label className="text-xs font-medium text-[var(--text-2)]">Type <span className="font-mono text-[var(--text)]">{projectName}</span> to confirm<input value={confirmation} onChange={(event) => setConfirmation(event.target.value)} className={fieldClass} autoComplete="off" /></label>{error && <p role="alert" className="mt-3 text-xs text-[var(--critical)]">{error}</p>}<div className="mt-4 flex justify-end"><button type="button" onClick={handleDelete} disabled={deleting || confirmation !== projectName} className="inline-flex items-center gap-2 rounded-[var(--radius-md)] bg-[var(--critical)] px-4 py-2.5 text-sm font-semibold text-white hover:brightness-110 disabled:cursor-not-allowed disabled:opacity-40"><Trash2 className="h-4 w-4" />{deleting ? 'Deleting...' : 'Delete project'}</button></div></div></section>
}
