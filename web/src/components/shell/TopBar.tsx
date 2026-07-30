import { Link, useMatches, useRouterState } from '@tanstack/react-router'
import { ArchiveRestore, Bell, LayoutGrid, Settings2 } from 'lucide-react'
import type { Session } from '../../api'
import { useTopologyStore } from '../../store/topology'
import type { ProjectTopology } from '../../types/resources'
import { OrcaLogo } from '../OrcaLogo'

type HealthTone = 'healthy' | 'warning' | 'critical' | 'unknown'

interface HealthPillProps {
  label: string
  tone: HealthTone
}

export function HealthPill({ label, tone }: HealthPillProps) {
  const color = {
    healthy: 'var(--healthy)',
    warning: 'var(--warning)',
    critical: 'var(--critical)',
    unknown: 'var(--text-3)',
  }[tone]

  return <span className="inline-flex items-center gap-2 text-xs text-[var(--text-2)]"><span className="h-1.5 w-1.5 rounded-full" style={{ backgroundColor: color }} />{label}</span>
}

interface TopBarProps {
  session: Session
  loggingOut: boolean
  onLogout: () => void
}

const navItems = [
  { label: 'Projects', href: '/' as const, icon: LayoutGrid },
  { label: 'Alerts', href: '/alerts' as const, icon: Bell },
  { label: 'Backups', href: '/backups' as const, icon: ArchiveRestore },
  { label: 'Settings', href: '/settings' as const, icon: Settings2 },
]

function isProjectTopology(value: unknown): value is ProjectTopology {
  return typeof value === 'object' && value !== null && 'project' in value && typeof value.project === 'object' && value.project !== null && 'name' in value.project && typeof value.project.name === 'string'
}

function sessionInitials(userID: string): string {
  const segments = userID.split(/[^a-zA-Z0-9]+/).filter(Boolean)
  if (segments.length > 1) return `${segments[0][0]}${segments[1][0]}`.toUpperCase()
  return userID.replace(/[^a-zA-Z0-9]/g, '').slice(0, 2).toUpperCase() || 'U'
}

function projectHealth(projectID: string | undefined, snapshot: ReturnType<typeof useTopologyStore.getState>['snapshot']): HealthPillProps {
  if (!projectID || snapshot?.project_id !== projectID || snapshot.clusters.length === 0) return { label: 'Unknown', tone: 'unknown' }
  if (snapshot.clusters.some((cluster) => cluster.health === 'down')) return { label: 'Critical', tone: 'critical' }
  if (snapshot.clusters.some((cluster) => cluster.health === 'degraded')) return { label: 'Degraded', tone: 'warning' }
  if (snapshot.clusters.some((cluster) => cluster.health === 'pending')) return { label: 'Pending', tone: 'warning' }
  if (snapshot.clusters.some((cluster) => cluster.health === 'unknown')) return { label: 'Unknown', tone: 'unknown' }
  return { label: 'Healthy', tone: 'healthy' }
}

export function TopBar({ session, loggingOut, onLogout }: TopBarProps) {
  const pathname = useRouterState({ select: (state) => state.location.pathname })
  const matches = useMatches()
  const snapshot = useTopologyStore((state) => state.snapshot)
  const projectMatch = matches.find((match) => match.routeId === '/_authenticated/projects/$projectId')
  const topology = isProjectTopology(projectMatch?.loaderData) ? projectMatch.loaderData : undefined
  const projectID = typeof projectMatch?.params.projectId === 'string' ? projectMatch.params.projectId : undefined
  const health = projectHealth(projectID, snapshot)
  const projectsActive = pathname === '/' || pathname.startsWith('/projects/')

  return (
    <>
      <header className="fixed inset-x-0 top-0 z-40 flex h-14 items-center justify-between border-b border-[var(--border)] bg-[var(--panel)] px-4 text-[var(--text)] sm:hidden">
        <Link to="/organizations" aria-label="Orca home" className="flex items-center gap-2 focus:outline-none focus:ring-2 focus:ring-[var(--accent)]">
          <OrcaLogo className="h-6 w-6" />
          <span className="text-sm font-semibold">Orca</span>
        </Link>
        <div className="flex items-center gap-3"><HealthPill {...health} /><UserMenu session={session} loggingOut={loggingOut} onLogout={onLogout} /></div>
      </header>

      <aside className="fixed left-4 top-1/2 z-40 hidden w-16 -translate-y-1/2 flex-col items-center rounded-full border border-[var(--border)] bg-[var(--panel)] px-2 py-3 text-[var(--text)] shadow-[0_16px_48px_rgba(0,0,0,0.4)] sm:flex">
        <Link to="/organizations" aria-label="Orca home" className="group relative grid h-10 w-10 place-items-center focus:outline-none focus:ring-2 focus:ring-[var(--accent)]"><OrcaLogo className="h-8 w-8" /><Tooltip>{topology ? topology.project.name : 'Orca home'}</Tooltip></Link>
        <nav aria-label="Primary navigation" className="my-3 flex flex-col gap-1.5 border-y border-[var(--border-soft)] py-3">
          {navItems.map((item) => {
            const active = item.href === '/' ? projectsActive : pathname === item.href
            const Icon = item.icon
            return <Link key={item.href} to={item.href} aria-label={item.label} className={`group relative grid h-10 w-10 place-items-center rounded-full ${active ? 'bg-[var(--accent)] text-[var(--accent-contrast)]' : 'text-[var(--text-2)] hover:bg-[var(--card)] hover:text-[var(--text)]'}`}><Icon aria-hidden="true" className="h-[18px] w-[18px]" strokeWidth={1.7} /><Tooltip>{item.label}</Tooltip></Link>
          })}
        </nav>
        <div className="group relative mb-1 grid h-7 w-7 place-items-center"><span className="h-2 w-2 rounded-full bg-[var(--healthy)]" /><Tooltip>Control plane operational</Tooltip></div>
        <button type="button" disabled={loggingOut} onClick={onLogout} aria-label="Log out" className="group relative grid h-9 w-9 place-items-center rounded-full border border-[var(--border)] bg-[var(--card)] font-mono text-[10px] text-[var(--text-2)] hover:border-[var(--accent)] disabled:opacity-50">{sessionInitials(session.user_id)}<Tooltip>{loggingOut ? 'Logging out...' : 'Log out'}</Tooltip></button>
      </aside>

      <nav aria-label="Mobile navigation" className="fixed inset-x-0 bottom-0 z-40 grid h-14 grid-cols-4 border-t border-[var(--border)] bg-[var(--panel)] sm:hidden">
        {navItems.map((item) => { const active = item.href === '/' ? projectsActive : pathname === item.href; const Icon = item.icon; return <Link key={item.href} to={item.href} className={`flex flex-col items-center justify-center gap-1 text-[10px] ${active ? 'text-[var(--accent)]' : 'text-[var(--text-3)]'}`}><Icon aria-hidden="true" className="h-4 w-4" strokeWidth={1.7} /><span>{item.label}</span></Link> })}
      </nav>
    </>
  )
}

function Tooltip({ children }: { children: string }) {
  return <span className="pointer-events-none absolute left-[calc(100%+12px)] top-1/2 z-50 hidden w-max -translate-y-1/2 rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--card)] px-2.5 py-1.5 text-xs text-[var(--text)] shadow-lg group-hover:block group-focus-visible:block">{children}</span>
}

function UserMenu({ session, loggingOut, onLogout }: TopBarProps) {
  return <details className="relative"><summary aria-label="Open user menu" className="grid h-8 w-8 cursor-pointer list-none place-items-center rounded-full border border-[var(--border)] bg-[var(--card)] font-mono text-[10px] [&::-webkit-details-marker]:hidden">{sessionInitials(session.user_id)}</summary><div className="absolute right-0 top-10 w-56 rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--card)] p-2 shadow-xl"><p className="truncate px-2 py-1.5 font-mono text-xs text-[var(--text-3)]">{session.user_id}</p><button type="button" disabled={loggingOut} onClick={onLogout} className="mt-1 w-full rounded-[var(--radius-sm)] px-2 py-2 text-left text-sm text-[var(--text-2)] hover:bg-[var(--panel)] hover:text-[var(--text)] disabled:opacity-60">{loggingOut ? 'Logging out...' : 'Log out'}</button></div></details>
}
