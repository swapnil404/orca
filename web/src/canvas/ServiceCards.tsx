import { motion } from 'framer-motion'
import { Bell, Blocks, DatabaseBackup, Network } from 'lucide-react'
import type { ComponentType } from 'react'
import { fadeUp, staggerContainer } from '../lib/motion'
import type { Cluster, ProjectClusterState } from '../types/resources'
import { extensionStatus, pgBackRestStatus, pgBouncerStatus, type NodeStatus } from './status'

type ServiceStatus = NodeStatus | 'disabled' | 'unavailable'
export type ServiceKind = 'pgbouncer' | 'pgbackrest' | 'extensions' | 'alerts'

interface ServiceCardsProps {
  clusters: Cluster[]
  states: ProjectClusterState[]
  now: number
  onSelect: (kind: ServiceKind, clusterID: string) => void
}

interface ServiceCardProps {
  title: string
  detail: string
  status: ServiceStatus
  icon: ComponentType<{ className?: string }>
  onClick: () => void
}

const statusStyles: Record<ServiceStatus, string> = {
  healthy: 'border-emerald-400/35 bg-emerald-400/10 text-emerald-300',
  degraded: 'border-amber-400/35 bg-amber-400/10 text-amber-200',
  down: 'border-rose-400/35 bg-rose-400/10 text-rose-300',
  pending: 'border-sky-400/35 bg-sky-400/10 text-sky-300',
  stale: 'border-orange-400/35 bg-orange-400/10 text-orange-200',
  unknown: 'border-slate-400/30 bg-slate-400/10 text-slate-300',
  disabled: 'border-slate-400/20 bg-slate-400/5 text-[var(--text-3)]',
  unavailable: 'border-slate-400/20 bg-slate-400/5 text-[var(--text-3)]',
}

function ServiceCard({ title, detail, status, icon: Icon, onClick }: ServiceCardProps) {
  return (
    <motion.button
      type="button"
      variants={fadeUp}
      onClick={onClick}
      className="group min-h-28 rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--card)] p-4 text-left transition-colors hover:border-[var(--text-3)] hover:bg-[var(--card-raised)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]"
    >
      <div className="flex items-start justify-between gap-3">
        <span className="grid h-8 w-8 place-items-center rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--panel)] text-[var(--text-2)]"><Icon className="h-4 w-4" /></span>
        <span className={`rounded-full border px-2 py-0.5 font-mono text-[9px] ${statusStyles[status]}`}>{status}</span>
      </div>
      <h3 className="mt-4 text-sm font-medium text-[var(--text)]">{title}</h3>
      <p className="mt-1 truncate text-xs text-[var(--text-3)]">{detail}</p>
    </motion.button>
  )
}

export function ServiceCards({ clusters, states, now, onSelect }: ServiceCardsProps) {
  return (
    <div className="space-y-4">
      {clusters.map((cluster) => {
        const state = states.find((candidate) => candidate.cluster_id === cluster.id)
        const pool = state?.actual_state?.pg_bouncer
        const backup = state?.actual_state?.backup
        const reportedExtensions = state?.actual_state?.enabled_extensions
        const connectionDetail = pool?.active_client_connections === undefined
          ? 'Connection count unavailable'
          : `${pool.active_client_connections} active connections`
        const extensionDetail = reportedExtensions === undefined
          ? `${cluster.enabled_extensions.length} desired · not reported`
          : `${cluster.enabled_extensions.length} desired · ${reportedExtensions.length} reported`

        return (
          <section key={cluster.id}>
            {clusters.length > 1 ? <p className="mb-2 font-mono text-[10px] uppercase tracking-[0.16em] text-[var(--text-3)]">{cluster.name} services</p> : null}
            <motion.div variants={staggerContainer} initial="hidden" animate="show" className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
              <ServiceCard title="PgBouncer" detail={cluster.pgbouncer_enabled ? connectionDetail : 'Not enabled'} status={cluster.pgbouncer_enabled ? pgBouncerStatus(state, pool, now) : 'disabled'} icon={Network} onClick={() => onSelect('pgbouncer', cluster.id)} />
              <ServiceCard title="pgBackRest" detail={cluster.pg_back_rest ? backup?.last_success_unix_seconds ? `Last success ${new Date(backup.last_success_unix_seconds * 1000).toLocaleString()}` : 'No successful backup reported' : 'Not enabled'} status={cluster.pg_back_rest ? pgBackRestStatus(state, backup, now) : 'disabled'} icon={DatabaseBackup} onClick={() => onSelect('pgbackrest', cluster.id)} />
              <ServiceCard title="Extensions" detail={extensionDetail} status={extensionStatus(state, cluster.enabled_extensions, now)} icon={Blocks} onClick={() => onSelect('extensions', cluster.id)} />
              <ServiceCard title="Alerts" detail="Alert state not in project telemetry" status="unavailable" icon={Bell} onClick={() => onSelect('alerts', cluster.id)} />
            </motion.div>
          </section>
        )
      })}
    </div>
  )
}
