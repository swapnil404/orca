import { createFileRoute } from '@tanstack/react-router'
import { useEffect, useState, type ReactNode } from 'react'
import { listGlobalAlerts, type AlertSeverity, type AlertStatus, type GlobalAlertIncident } from '../../api/global-alerts'

export const Route = createFileRoute('/_authenticated/alerts')({
  ssr: false,
  loader: () => listGlobalAlerts(),
  component: AlertsPage,
})

const comparisonLabels: Record<GlobalAlertIncident['comparison'], string> = {
  gt: '>',
  gte: '>=',
  lt: '<',
  lte: '<=',
  eq: '=',
  neq: '!=',
}

function AlertsPage() {
  const initialAlerts = Route.useLoaderData()
  const [alerts, setAlerts] = useState(initialAlerts)
  const [refreshFailed, setRefreshFailed] = useState(false)
  const [projectID, setProjectID] = useState('all')
  const [severity, setSeverity] = useState<AlertSeverity | 'all'>('all')
  const [status, setStatus] = useState<AlertStatus>('firing')
  const projects = Array.from(new Map(alerts.map((alert) => [alert.project_id, alert.project_name])).entries())
  const filteredAlerts = alerts.filter((alert) => {
    const alertStatus: AlertStatus = alert.resolved_at ? 'resolved' : 'firing'
    return (projectID === 'all' || alert.project_id === projectID)
      && (severity === 'all' || alert.severity === severity)
      && alertStatus === status
  })

  useEffect(() => {
    let active = true
    const refresh = () => void listGlobalAlerts().then((nextAlerts) => {
      if (!active) return
      setAlerts(nextAlerts)
      setRefreshFailed(false)
    }).catch(() => {
      if (active) setRefreshFailed(true)
    })
    const timer = window.setInterval(refresh, 10_000)
    return () => { active = false; window.clearInterval(timer) }
  }, [])

  return (
    <main className="min-h-[calc(100vh-56px)] px-4 py-6 text-[var(--text)] sm:px-6 lg:px-8">
      <div className="mx-auto max-w-7xl">
        <header>
          <p className="font-mono text-[10px] font-medium uppercase tracking-[0.16em] text-[var(--text-3)]">Incident overview</p>
          <h1 className="mt-1.5 text-xl font-semibold">Alerts</h1>
          <p className="mt-1 text-sm text-[var(--text-2)]">Alert activity across every project you can access. State refreshes every ten seconds.</p>
          {refreshFailed && <p role="status" className="mt-2 text-xs text-[var(--warning)]">Alert refresh failed. Displayed state may be stale.</p>}
        </header>
        <div className="my-5 flex flex-wrap gap-3">
          <Filter label="Project" value={projectID} onChange={setProjectID}>
            <option value="all">All projects</option>
            {projects.map(([id, name]) => <option key={id} value={id}>{name}</option>)}
          </Filter>
          <Filter label="Severity" value={severity} onChange={(value) => setSeverity(value as AlertSeverity | 'all')}>
            <option value="all">All severities</option>
            <option value="critical">Critical</option>
            <option value="warning">Warning</option>
            <option value="info">Info</option>
          </Filter>
          <Filter label="Status" value={status} onChange={(value) => setStatus(value as AlertStatus)}>
            <option value="firing">Firing</option>
            <option value="resolved">Resolved</option>
          </Filter>
          <span className="self-center font-mono text-xs text-[var(--text-3)]">{filteredAlerts.length} alerts</span>
        </div>

        {filteredAlerts.length > 0
          ? <AlertTable alerts={filteredAlerts} />
          : <EmptyState status={status} filtered={projectID !== 'all' || severity !== 'all'} />}
      </div>
    </main>
  )
}

interface FilterProps {
  label: string
  value: string
  onChange: (value: string) => void
  children: ReactNode
}

function Filter({ label, value, onChange, children }: FilterProps) {
  return <label className="text-xs font-medium text-[var(--text-2)]">{label}<select value={value} onChange={(event) => onChange(event.target.value)} className="ml-2 rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--panel)] px-3 py-2 text-sm text-[var(--text)]">{children}</select></label>
}

function AlertTable({ alerts }: { alerts: GlobalAlertIncident[] }) {
  return (
    <div className="overflow-x-auto rounded-[var(--radius-sm)] border border-[var(--border)]">
      <table className="w-full min-w-[760px] border-collapse text-left text-sm">
        <thead className="bg-[var(--panel)] text-xs font-medium text-[var(--text-2)]"><tr><th className="px-4 py-3">Project</th><th className="px-4 py-3">Rule</th><th className="px-4 py-3">Severity</th><th className="px-4 py-3">Status</th><th className="px-4 py-3">Fired at</th></tr></thead>
        <tbody className="divide-y divide-[var(--border-soft)] bg-[var(--card)]">
          {alerts.map((alert) => <tr key={alert.id} className="hover:bg-[var(--card-raised)]"><td className="px-4 py-3"><a href={`/projects/${encodeURIComponent(alert.project_id)}/alerts`} className="font-medium text-[var(--text)] hover:underline">{alert.project_name}</a></td><td className="px-4 py-3"><div className="font-medium text-[var(--text)]">{alert.metric_name}</div><div className="mt-0.5 font-mono text-[11px] text-[var(--text-3)]">{comparisonLabels[alert.comparison]} {alert.threshold}</div></td><td className="px-4 py-3"><SeverityBadge severity={alert.severity} /></td><td className="px-4 py-3"><StatusBadge firing={!alert.resolved_at} /></td><td className="px-4 py-3 font-mono text-xs text-[var(--text-2)]">{new Date(alert.fired_at).toLocaleString()}</td></tr>)}
        </tbody>
      </table>
    </div>
  )
}

function SeverityBadge({ severity }: { severity: AlertSeverity }) {
  const colors = { critical: 'var(--critical)', warning: 'var(--warning)', info: 'var(--accent)' }
  return <span className="inline-flex items-center gap-2 capitalize text-xs text-[var(--text-2)]"><span className="h-1.5 w-1.5 rounded-full" style={{ backgroundColor: colors[severity] }} />{severity}</span>
}

function StatusBadge({ firing }: { firing: boolean }) {
  return <span className={`inline-flex rounded-full border px-2 py-0.5 text-[11px] font-medium ${firing ? 'border-[var(--critical)] text-[var(--critical)]' : 'border-[var(--border)] text-[var(--text-2)]'}`}>{firing ? 'Firing' : 'Resolved'}</span>
}

function EmptyState({ status, filtered }: { status: AlertStatus; filtered: boolean }) {
  const title = status === 'firing' && !filtered ? 'No alerts are firing' : `No ${status} alerts match these filters`
  const description = status === 'firing' && !filtered ? 'Every accessible project is currently clear.' : 'Try selecting a different project, severity, or status.'
  return <section className="rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--card)] p-8"><h2 className="text-base font-semibold">{title}</h2><p className="mt-2 text-sm text-[var(--text-2)]">{description}</p></section>
}
