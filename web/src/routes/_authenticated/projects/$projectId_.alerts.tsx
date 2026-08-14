import { Link, createFileRoute } from '@tanstack/react-router'
import { BellRing, CircleCheck, Siren } from 'lucide-react'
import { useEffect, useState } from 'react'
import { getProjectAlerts, getProjectTopology } from '../../../api'
import { alertRuleLabel } from '../../../lib/alerts'
import type { AlertSeverity, ProjectAlerts } from '../../../types/alerts'
import type { ProjectTopology } from '../../../types/resources'

type HistoryStatus = 'all' | 'firing' | 'resolved'
type SeverityFilter = 'all' | AlertSeverity

export const Route = createFileRoute('/_authenticated/projects/$projectId_/alerts')({
  ssr: false,
  loader: async ({ params }) => {
    const [topology, alerts] = await Promise.all([
      getProjectTopology(params.projectId),
      getProjectAlerts(params.projectId),
    ])
    return { ...topology, ...alerts }
  },
  component: ProjectAlertsRoute,
})

function formatDate(value: string): string {
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))
}

function severityClass(severity: AlertSeverity): string {
  if (severity === 'critical') return 'border-rose-400/30 bg-rose-400/10 text-rose-300'
  if (severity === 'warning') return 'border-amber-400/30 bg-amber-400/10 text-amber-200'
  return 'border-sky-400/30 bg-sky-400/10 text-sky-200'
}

function ProjectAlertsRoute() {
  const initial = Route.useLoaderData()
  const { projectId } = Route.useParams()
  if (initial.project.id !== projectId) return null
  return <ProjectAlertsPage key={projectId} projectId={projectId} initial={initial} />
}

function ProjectAlertsPage({ projectId, initial }: { projectId: string; initial: ProjectTopology & ProjectAlerts }) {
  const { project, clusters } = initial
  const [alerts, setAlerts] = useState<ProjectAlerts>({ rules: initial.rules, incidents: initial.incidents })
  const [refreshFailed, setRefreshFailed] = useState(false)
  const [status, setStatus] = useState<HistoryStatus>('all')
  const [severity, setSeverity] = useState<SeverityFilter>('all')
  const { rules, incidents } = alerts

  useEffect(() => {
    let active = true
    const refresh = () => void getProjectAlerts(projectId).then((nextAlerts) => {
      if (!active) return
      setAlerts(nextAlerts)
      setRefreshFailed(false)
    }).catch(() => {
      if (active) setRefreshFailed(true)
    })
    const timer = window.setInterval(refresh, 10_000)
    return () => { active = false; window.clearInterval(timer) }
  }, [projectId])

  const clusterNames = new Map(clusters.map((cluster) => [cluster.id, cluster.name]))
  const filteredIncidents = incidents.filter((incident) => {
    const incidentStatus = incident.resolved_at === null ? 'firing' : 'resolved'
    return (status === 'all' || status === incidentStatus) && (severity === 'all' || severity === incident.severity)
  })
  const firingCount = incidents.filter((incident) => incident.resolved_at === null).length

  return (
    <main className="min-h-[calc(100vh-56px)] px-4 py-6 text-[var(--text)] sm:px-6 lg:px-8">
      <div className="mx-auto max-w-7xl">
        <header className="flex flex-col justify-between gap-4 border-b border-[var(--border)] pb-6 sm:flex-row sm:items-end">
          <div>
            <div className="flex items-center gap-2 text-xs text-[var(--text-3)]">
              <Link to="/" className="hover:text-[var(--text)]">Projects</Link>
              <span>/</span>
              <Link to="/projects/$projectId" params={{ projectId: project.id }} search={{ tab: 'topology', restore: undefined }} className="hover:text-[var(--text)]">{project.name}</Link>
            </div>
            <h1 className="mt-3 text-2xl font-semibold tracking-tight">Alerts</h1>
            <p className="mt-1 text-sm text-[var(--text-2)]">Configured thresholds and their recorded firing history. Alert state refreshes every ten seconds.</p>
            {refreshFailed && <p role="status" className="mt-2 text-xs text-[var(--warning)]">Alert refresh failed. Displayed state may be stale.</p>}
          </div>
          <div className="flex gap-2">
            <Summary label="Active rules" value={rules.length} />
            <Summary label="Firing now" value={firingCount} critical={firingCount > 0} />
          </div>
        </header>

        <section className="mt-8">
          <div className="flex items-center gap-3">
            <BellRing className="h-4 w-4 text-[var(--accent)]" />
            <div><h2 className="text-base font-semibold">Active alert rules</h2><p className="text-xs text-[var(--text-3)]">Read-only alert state; the current API does not support rule changes.</p></div>
          </div>
          {rules.length > 0 ? (
            <div className="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-3">
              {rules.map((rule) => (
                <article key={rule.id} className="rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--card)] p-4">
                  <div className="flex items-start justify-between gap-3">
                    <span className={`rounded-full border px-2 py-0.5 font-mono text-[10px] uppercase ${severityClass(rule.severity)}`}>{rule.severity}</span>
                    <span className={`flex items-center gap-1.5 text-xs ${rule.current_state === 'firing' ? 'text-[var(--critical)]' : 'text-[var(--healthy)]'}`}><span className="h-1.5 w-1.5 rounded-full bg-current" />{rule.current_state === 'firing' ? 'Firing' : 'OK'}</span>
                  </div>
                  <h3 className="mt-4 break-words font-mono text-sm text-[var(--text)]">{alertRuleLabel(rule)}</h3>
                  <p className="mt-2 text-xs text-[var(--text-3)]">{rule.cluster_id ? clusterNames.get(rule.cluster_id) ?? rule.cluster_id : 'All project clusters'}</p>
                  <p className="mt-4 border-t border-[var(--border-soft)] pt-3 text-[11px] text-[var(--text-3)]">Fires after <span className="font-mono text-[var(--text-2)]">{rule.duration_before_firing_seconds}s</span></p>
                </article>
              ))}
            </div>
          ) : <EmptyState icon={BellRing} title="No alert rules" detail="This project does not have any configured alert rules." />}
        </section>

        <section className="mt-10">
          <div className="flex flex-col justify-between gap-4 sm:flex-row sm:items-end">
            <div className="flex items-center gap-3"><Siren className="h-4 w-4 text-[var(--text-2)]" /><div><h2 className="text-base font-semibold">Firing and resolved history</h2><p className="text-xs text-[var(--text-3)]">Every state transition recorded by the evaluator.</p></div></div>
            <div className="flex flex-wrap gap-2">
              <Filter label="Status" value={status} onChange={(value) => setStatus(value as HistoryStatus)} options={['all', 'firing', 'resolved']} />
              <Filter label="Severity" value={severity} onChange={(value) => setSeverity(value as SeverityFilter)} options={['all', 'critical', 'warning', 'info']} />
            </div>
          </div>

          {incidents.length === 0 ? <EmptyState icon={CircleCheck} title="No alert history" detail="No alert rule has fired for this project." /> : filteredIncidents.length === 0 ? (
            <div className="mt-4 rounded-[var(--radius-md)] border border-dashed border-[var(--border)] px-6 py-10 text-center text-sm text-[var(--text-3)]">No incidents match these filters.</div>
          ) : (
            <div className="mt-4 overflow-x-auto rounded-[var(--radius-md)] border border-[var(--border)]">
              <table className="w-full min-w-[760px] border-collapse text-left text-sm">
                <thead className="bg-[var(--panel)] text-xs text-[var(--text-2)]"><tr><th className="px-4 py-3 font-medium">Rule</th><th className="px-4 py-3 font-medium">Severity</th><th className="px-4 py-3 font-medium">Fired at</th><th className="px-4 py-3 font-medium">Resolved at</th></tr></thead>
                <tbody className="divide-y divide-[var(--border-soft)] bg-[var(--card)]">
                  {filteredIncidents.map((incident) => <tr key={incident.id} className="hover:bg-[var(--card-raised)]"><td className="px-4 py-3"><div className="font-mono text-xs text-[var(--text)]">{alertRuleLabel(incident)}</div><div className="mt-1 font-mono text-[10px] text-[var(--text-3)]">{incident.rule_id}</div></td><td className="px-4 py-3"><span className={`rounded-full border px-2 py-0.5 font-mono text-[10px] uppercase ${severityClass(incident.severity)}`}>{incident.severity}</span></td><td className="px-4 py-3 font-mono text-xs text-[var(--text-2)]">{formatDate(incident.fired_at)}</td><td className={`px-4 py-3 font-mono text-xs ${incident.resolved_at ? 'text-[var(--text-2)]' : 'text-[var(--critical)]'}`}>{incident.resolved_at ? formatDate(incident.resolved_at) : 'still firing'}</td></tr>)}
                </tbody>
              </table>
            </div>
          )}
        </section>
      </div>
    </main>
  )
}

interface SummaryProps { label: string; value: number; critical?: boolean }
function Summary({ label, value, critical = false }: SummaryProps) {
  return <div className="min-w-24 rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--panel)] px-3 py-2"><div className={`font-mono text-lg ${critical ? 'text-[var(--critical)]' : 'text-[var(--text)]'}`}>{value}</div><div className="text-[10px] uppercase tracking-wide text-[var(--text-3)]">{label}</div></div>
}

interface FilterProps { label: string; value: string; options: string[]; onChange: (value: string) => void }
function Filter({ label, value, options, onChange }: FilterProps) {
  return <label className="flex items-center gap-2 rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--panel)] px-3 py-2 text-xs text-[var(--text-3)]">{label}<select value={value} onChange={(event) => onChange(event.target.value)} className="bg-transparent font-medium capitalize text-[var(--text)] outline-none">{options.map((option) => <option key={option} value={option} className="bg-[var(--panel)]">{option}</option>)}</select></label>
}

interface EmptyStateProps { icon: typeof BellRing; title: string; detail: string }
function EmptyState({ icon: Icon, title, detail }: EmptyStateProps) {
  return <div className="mt-4 rounded-[var(--radius-md)] border border-dashed border-[var(--border)] px-6 py-10 text-center"><Icon className="mx-auto h-5 w-5 text-[var(--text-3)]" /><h3 className="mt-3 text-sm font-medium">{title}</h3><p className="mt-1 text-xs text-[var(--text-3)]">{detail}</p></div>
}
