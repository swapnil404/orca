import { Link, useMatches, useRouterState } from '@tanstack/react-router'
import { motion } from 'framer-motion'
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

  return (
    <span className="inline-flex items-center gap-2 rounded-full border border-[var(--border)] bg-black/20 px-2.5 py-1 text-[11px] font-medium text-[var(--text-2)] shadow-[inset_0_1px_rgba(255,255,255,0.03)]">
      <span aria-hidden="true" className="relative h-1.5 w-1.5 rounded-full" style={{ backgroundColor: color }} />
      {label}
    </span>
  )
}

interface TopBarProps {
  session: Session
  loggingOut: boolean
  onLogout: () => void
}

interface NavItem {
  label: string
  href: '/' | '/alerts' | '/backups' | '/settings'
  icon: 'projects' | 'alerts' | 'backups' | 'settings'
}

const navItems: NavItem[] = [
  { label: 'Projects', href: '/', icon: 'projects' },
  { label: 'Alerts', href: '/alerts', icon: 'alerts' },
  { label: 'Backups', href: '/backups', icon: 'backups' },
  { label: 'Settings', href: '/settings', icon: 'settings' },
]

function isProjectTopology(value: unknown): value is ProjectTopology {
  return (
    typeof value === 'object' &&
    value !== null &&
    'project' in value &&
    typeof value.project === 'object' &&
    value.project !== null &&
    'name' in value.project &&
    typeof value.project.name === 'string'
  )
}

function sessionInitials(userID: string): string {
  const segments = userID.split(/[^a-zA-Z0-9]+/).filter(Boolean)
  if (segments.length > 1) {
    return `${segments[0][0]}${segments[1][0]}`.toUpperCase()
  }
  return userID.replace(/[^a-zA-Z0-9]/g, '').slice(0, 2).toUpperCase() || 'U'
}

function projectHealth(projectID: string | undefined, snapshot: ReturnType<typeof useTopologyStore.getState>['snapshot']): HealthPillProps {
  if (!projectID || snapshot?.project_id !== projectID || snapshot.clusters.length === 0) {
    return { label: 'Unknown', tone: 'unknown' }
  }
  if (snapshot.clusters.some((cluster) => cluster.health === 'down')) {
    return { label: 'Critical', tone: 'critical' }
  }
  if (snapshot.clusters.some((cluster) => cluster.health === 'degraded')) {
    return { label: 'Degraded', tone: 'warning' }
  }
  if (snapshot.clusters.some((cluster) => cluster.health === 'pending')) {
    return { label: 'Pending', tone: 'warning' }
  }
  if (snapshot.clusters.some((cluster) => cluster.health === 'unknown')) {
    return { label: 'Unknown', tone: 'unknown' }
  }
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
      <header className="fixed inset-x-0 top-0 z-40 flex h-16 items-center justify-between border-b border-[var(--border-soft)] bg-[var(--bg)] px-4 font-[var(--font-sans)] text-[var(--text)] sm:hidden">
        <Link to="/" aria-label="Orca home" className="flex items-center gap-2 rounded-[var(--radius-sm)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]">
          <OrcaLogo className="h-7 w-7" />
          <span className="text-[15px] font-semibold tracking-[-0.02em]">orca</span>
        </Link>
        <div className="flex items-center gap-2">
          <HealthPill {...health} />
          <UserMenu session={session} loggingOut={loggingOut} onLogout={onLogout} />
        </div>
      </header>

      <aside className="fixed left-4 top-1/2 z-40 hidden w-16 -translate-y-1/2 flex-col items-center rounded-full border border-[var(--border)] bg-[var(--panel)] px-2 py-3 font-[var(--font-sans)] text-[var(--text)] shadow-[0_24px_70px_rgba(0,0,0,0.42)] sm:flex">
        <Link to="/" aria-label="Orca home" className="group relative grid h-11 w-11 shrink-0 place-items-center rounded-full focus:outline-none focus:ring-2 focus:ring-[var(--accent)]">
          <OrcaLogo className="h-9 w-9" />
          <RailTooltip>{topology ? topology.project.name : 'Orca home'}</RailTooltip>
        </Link>

        <nav aria-label="Primary navigation" className="my-3 flex flex-col gap-2 border-y border-[var(--border-soft)] py-3">
          {navItems.map((item) => {
            const active = item.href === '/' ? projectsActive : pathname === item.href || pathname.startsWith(`${item.href}/`)
            return (
              <Link key={item.href} to={item.href} aria-label={item.label} className={`group relative grid h-11 w-11 place-items-center rounded-full transition-colors duration-300 ${active ? 'text-[var(--accent-contrast)]' : 'text-[var(--text-2)] hover:bg-[var(--card-raised)] hover:text-[var(--text)]'}`}>
                {active && <motion.span layoutId="sidebar-active" className="absolute inset-0 rounded-full bg-[var(--accent)]" transition={{ type: 'spring', stiffness: 260, damping: 28, mass: 0.8 }} />}
                <span className="relative"><NavIcon name={item.icon} /></span>
                <RailTooltip>{item.label}</RailTooltip>
              </Link>
            )
          })}
        </nav>

        <div className="flex flex-col items-center gap-2">
          <div className="group relative grid h-8 w-8 place-items-center"><span className="relative h-2 w-2 rounded-full bg-[var(--healthy)]" /><RailTooltip>Control plane operational</RailTooltip></div>
          <button type="button" disabled={loggingOut} onClick={onLogout} aria-label="Log out" className="group relative grid h-10 w-10 place-items-center rounded-full border border-[var(--border)] bg-[var(--card)] font-mono text-[10px] font-semibold text-[var(--text-2)] transition-all duration-300 hover:border-[var(--accent)] hover:text-[var(--text)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)] disabled:opacity-50">
            {sessionInitials(session.user_id)}
            <RailTooltip>{loggingOut ? 'Logging out...' : 'Log out'}</RailTooltip>
          </button>
        </div>
      </aside>

      <nav aria-label="Mobile navigation" className="fixed inset-x-0 bottom-0 z-40 grid h-16 grid-cols-4 border-t border-[var(--border)] bg-[var(--panel)] px-2 sm:hidden">
        {navItems.map((item) => {
          const active = item.href === '/' ? projectsActive : pathname === item.href || pathname.startsWith(`${item.href}/`)
          return <Link key={item.href} to={item.href} className={`relative flex flex-col items-center justify-center gap-1 text-[9px] font-medium transition-colors duration-300 ${active ? 'text-[var(--accent)]' : 'text-[var(--text-3)]'}`}><NavIcon name={item.icon} /><span>{item.label}</span>{active && <motion.span layoutId="mobile-active" className="absolute top-0 h-0.5 w-7 rounded-full bg-[var(--accent)]" transition={{ type: 'spring', stiffness: 260, damping: 28, mass: 0.8 }} />}</Link>
        })}
      </nav>
    </>
  )
}

function RailTooltip({ children }: { children: string }) {
  return <span className="pointer-events-none absolute left-[calc(100%+14px)] top-1/2 z-50 w-max -translate-x-1 -translate-y-1/2 rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--card-raised)] px-3 py-2 text-xs font-medium text-[var(--text)] opacity-0 shadow-xl transition-all duration-300 ease-[var(--ease-premium)] group-hover:translate-x-0 group-hover:opacity-100 group-focus-visible:translate-x-0 group-focus-visible:opacity-100">{children}</span>
}

type UserMenuProps = TopBarProps

function UserMenu({ session, loggingOut, onLogout }: UserMenuProps) {
  return (
    <details className="group relative">
      <summary title={`User ${session.user_id}`} aria-label="Open user menu" className="grid h-8 w-8 cursor-pointer list-none place-items-center rounded-full border border-[var(--border)] bg-[var(--card-raised)] font-mono text-[10px] font-semibold focus:outline-none focus:ring-2 focus:ring-[var(--accent)] [&::-webkit-details-marker]:hidden">{sessionInitials(session.user_id)}</summary>
      <div className="absolute right-0 top-11 w-56 rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--card)] p-2 shadow-[0_24px_70px_rgba(0,0,0,0.45)]">
        <p className="truncate px-2 py-1.5 font-mono text-xs text-[var(--text-3)]">{session.user_id}</p>
        <button type="button" disabled={loggingOut} onClick={onLogout} className="mt-1 w-full rounded-[var(--radius-sm)] px-2 py-2 text-left text-sm text-[var(--text-2)] transition hover:bg-[var(--panel)] hover:text-[var(--text)] disabled:opacity-60">{loggingOut ? 'Logging out...' : 'Log out'}</button>
      </div>
    </details>
  )
}

function NavIcon({ name }: { name: NavItem['icon'] }) {
  const icons = {
    projects: LayoutGrid,
    alerts: Bell,
    backups: ArchiveRestore,
    settings: Settings2,
  }
  const Icon = icons[name]
  return <Icon aria-hidden="true" className="h-[18px] w-[18px]" strokeWidth={1.7} />
}
