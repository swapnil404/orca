import { Link, useMatches, useRouterState } from '@tanstack/react-router'
import type { Session } from '../../api'
import { useTopologyStore } from '../../store/topology'
import type { ProjectTopology } from '../../types/resources'

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
    <span className="inline-flex items-center gap-2 rounded-full border border-[var(--border)] bg-[var(--panel)] px-2.5 py-1 text-xs font-medium text-[var(--text-2)]">
      <span aria-hidden="true" className="h-1.5 w-1.5 rounded-full" style={{ backgroundColor: color }} />
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
  href: string
}

const navItems: NavItem[] = [
  { label: 'Alerts', href: '/alerts' },
  { label: 'Backups', href: '/backups' },
  { label: 'Settings', href: '/settings' },
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
    <header className="sticky top-0 z-40 h-[52px] border-b border-[var(--border)] bg-[var(--card)] font-[var(--font-sans)] text-[var(--text)]">
      <div className="flex h-full items-stretch gap-3 px-4 sm:px-6">
        <Link to="/" aria-label="Orca home" className="flex shrink-0 items-center gap-2 self-center rounded-[var(--radius-sm)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]">
          <span aria-hidden="true" className="h-5 w-5 rounded-[5px] bg-[var(--accent)]" />
          <span className="text-sm font-semibold tracking-[-0.02em]">orca</span>
        </Link>

        <nav aria-label="Primary navigation" className="flex min-w-0 flex-1 items-stretch gap-1 overflow-x-auto sm:ml-3 sm:gap-2">
          {topology ? (
            <details className="group relative flex shrink-0 items-stretch">
              <summary className={`flex cursor-pointer list-none items-center gap-1.5 border-b-2 px-2 text-sm font-medium transition-colors duration-[var(--dur-fast)] [&::-webkit-details-marker]:hidden ${projectsActive ? 'border-[var(--accent)] text-[var(--text)]' : 'border-transparent text-[var(--text-2)]'}`}>
                <span className="max-w-32 truncate">{topology.project.name}</span>
                <svg aria-hidden="true" viewBox="0 0 16 16" className="h-3.5 w-3.5 fill-none stroke-current transition-transform duration-[var(--dur-fast)] group-open:rotate-180"><path d="m4 6 4 4 4-4" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" /></svg>
              </summary>
              <div className="absolute left-0 top-[46px] w-56 rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--card)] p-2 shadow-[0_16px_40px_rgba(0,0,0,0.35)]">
                <p className="px-2 py-1 text-[11px] font-medium uppercase tracking-[0.12em] text-[var(--text-3)]">Current project</p>
                <p className="truncate rounded-[var(--radius-sm)] bg-[var(--panel)] px-2 py-2 text-sm text-[var(--text)]">{topology.project.name}</p>
                <p className="px-2 pt-2 text-xs text-[var(--text-3)]">Project switching coming soon</p>
              </div>
            </details>
          ) : (
            <Link to="/" className={`flex shrink-0 items-center border-b-2 px-2 text-sm font-medium transition-colors duration-[var(--dur-fast)] ${projectsActive ? 'border-[var(--accent)] text-[var(--text)]' : 'border-transparent text-[var(--text-2)] hover:text-[var(--text)]'}`}>
              Projects
            </Link>
          )}
          {navItems.map((item) => {
            const active = pathname === item.href || pathname.startsWith(`${item.href}/`)
            return (
              <a key={item.href} href={item.href} className={`flex shrink-0 items-center border-b-2 px-2 text-sm font-medium transition-colors duration-[var(--dur-fast)] ${active ? 'border-[var(--accent)] text-[var(--text)]' : 'border-transparent text-[var(--text-2)] hover:text-[var(--text)]'}`}>
                {item.label}
              </a>
            )
          })}
        </nav>

        <div className="flex shrink-0 items-center gap-2 sm:gap-3">
          <HealthPill {...health} />
          <details className="group relative">
            <summary title={`User ${session.user_id}`} aria-label="Open user menu" className="grid h-8 w-8 cursor-pointer list-none place-items-center rounded-full border border-[var(--border)] bg-[var(--panel)] font-mono text-[11px] font-semibold text-[var(--text)] transition-colors duration-[var(--dur-fast)] hover:border-[var(--text-3)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)] [&::-webkit-details-marker]:hidden">
              {sessionInitials(session.user_id)}
            </summary>
            <div className="absolute right-0 top-10 w-52 rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--card)] p-2 shadow-[0_16px_40px_rgba(0,0,0,0.35)]">
              <p className="truncate px-2 py-1.5 font-mono text-xs text-[var(--text-3)]" title={session.user_id}>{session.user_id}</p>
              <button type="button" disabled={loggingOut} onClick={onLogout} className="mt-1 w-full rounded-[var(--radius-sm)] px-2 py-2 text-left text-sm text-[var(--text-2)] transition-colors duration-[var(--dur-fast)] hover:bg-[var(--panel)] hover:text-[var(--text)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)] disabled:cursor-not-allowed disabled:opacity-60">
                {loggingOut ? 'Logging out...' : 'Log out'}
              </button>
            </div>
          </details>
        </div>
      </div>
    </header>
  )
}
