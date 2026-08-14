import { Link, useMatches, useNavigate, useRouterState } from '@tanstack/react-router'
import { ArchiveRestore, Bell, Building2, Check, ChevronDown, Database, LayoutGrid, Plus, Search, Settings2 } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import type { Session } from '../../api'
import { primaryStatus } from '../../canvas/status'
import { useTopologyStore } from '../../store/topology'
import type { Organization } from '../../types/organizations'
import type { Project, ProjectTopology } from '../../types/resources'
import { OrcaLogo } from '../OrcaLogo'

type HealthTone = 'healthy' | 'warning' | 'critical' | 'unknown'

interface HealthPillProps {
  label: string
  tone: HealthTone
}

export function HealthPill({ label, tone }: HealthPillProps) {
  return <span className="inline-flex items-center gap-2 text-xs text-[var(--text-2)]"><span className="h-1.5 w-1.5 rounded-full" style={{ backgroundColor: healthColor(tone) }} />{label}</span>
}

function healthColor(tone: HealthTone): string {
  return { healthy: 'var(--healthy)', warning: 'var(--warning)', critical: 'var(--critical)', unknown: 'var(--text-3)' }[tone]
}

interface TopBarProps {
  session: Session
  organizations: Organization[]
  projects: Project[]
  loggingOut: boolean
  onLogout: () => void
  onOpenSearch: () => void
}

const navItems = [
  { label: 'Organizations', href: '/organizations' as const, icon: Building2 },
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

function projectHealth(projectID: string | undefined, snapshot: ReturnType<typeof useTopologyStore.getState>['snapshot'], now: number): HealthPillProps {
  if (!projectID || snapshot?.project_id !== projectID || snapshot.clusters.length === 0) return { label: 'Unknown', tone: 'unknown' }
  const statuses = snapshot.clusters.map((cluster) => primaryStatus(cluster, now))
  if (statuses.includes('down')) return { label: 'Critical', tone: 'critical' }
  if (statuses.includes('degraded')) return { label: 'Degraded', tone: 'warning' }
  if (statuses.includes('stale')) return { label: 'Stale', tone: 'warning' }
  if (statuses.includes('unknown')) return { label: 'Unknown', tone: 'unknown' }
  return { label: 'Healthy', tone: 'healthy' }
}

export function TopBar({ session, organizations, projects, loggingOut, onLogout, onOpenSearch }: TopBarProps) {
  const navigate = useNavigate()
  const location = useRouterState({ select: (state) => state.location })
  const matches = useMatches()
  const snapshot = useTopologyStore((state) => state.snapshot)
  const [now, setNow] = useState(() => Date.now())

  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 15_000)
    return () => window.clearInterval(timer)
  }, [])

  useEffect(() => {
    document.querySelectorAll<HTMLDetailsElement>('header details[open]').forEach((details) => details.removeAttribute('open'))
  }, [location.href])

  useEffect(() => {
    function closeMenus(event: PointerEvent) {
      if (event.target instanceof Element && event.target.closest('header details')) return
      document.querySelectorAll<HTMLDetailsElement>('header details[open]').forEach((details) => details.removeAttribute('open'))
    }

    function closeOtherMenus(event: Event) {
      if (!(event.target instanceof HTMLDetailsElement) || !event.target.open || !event.target.closest('header')) return
      document.querySelectorAll<HTMLDetailsElement>('header details[open]').forEach((details) => {
        if (details !== event.target) details.removeAttribute('open')
      })
    }

    document.addEventListener('pointerdown', closeMenus)
    document.addEventListener('toggle', closeOtherMenus, true)
    return () => {
      document.removeEventListener('pointerdown', closeMenus)
      document.removeEventListener('toggle', closeOtherMenus, true)
    }
  }, [])

  const projectMatch = [...matches].reverse().find((match) => 'projectId' in match.params && typeof match.params.projectId === 'string')
  const topology = isProjectTopology(projectMatch?.loaderData) ? projectMatch.loaderData : undefined
  const projectID = projectMatch && 'projectId' in projectMatch.params && typeof projectMatch.params.projectId === 'string' ? projectMatch.params.projectId : undefined
  const searchOrganizationID = typeof location.search.organizationId === 'string' ? location.search.organizationId : undefined
  const activeOrganizationID = topology?.project.organization_id ?? searchOrganizationID ?? organizations[0]?.id ?? ''
  const organizationProjects = projects.filter((project) => project.organization_id === activeOrganizationID)
  const health = projectHealth(projectID, snapshot, now)

  return (
    <>
    <header className="fixed inset-x-0 top-0 z-40 h-28 border-b border-[var(--border)] bg-[color:rgba(17,17,18,0.94)] text-[var(--text)] backdrop-blur-xl md:h-16">
      <div className="mx-auto flex h-16 max-w-[1600px] items-center gap-3 px-3 sm:px-5">
        <Link to="/" search={{ organizationId: activeOrganizationID || undefined }} aria-label="Projects home" className="group flex shrink-0 items-center gap-2.5 border-r border-[var(--border)] pr-3 sm:pr-4"><span className="grid h-9 w-9 place-items-center rounded-[var(--radius-sm)] bg-[var(--accent)] transition-transform duration-200 group-hover:-rotate-3"><OrcaLogo className="h-7 w-7 [&_path:first-of-type]:fill-[var(--accent-contrast)] [&_path:last-of-type]:fill-[var(--panel)]" /></span><span className="hidden leading-none lg:block"><strong className="block text-sm font-semibold tracking-[-0.02em]">orca</strong><span className="mt-1 block font-mono text-[8px] uppercase tracking-[0.18em] text-[var(--text-3)]">control plane</span></span></Link>

        <OrganizationSwitcher organizations={organizations} activeOrganizationID={activeOrganizationID} onSelect={(organizationID) => void navigate({ to: '/', search: { organizationId: organizationID } })} />
        <Link to="/organizations" search={{ create: true }} aria-label="New organization" title="New organization" className="hidden h-8 w-8 shrink-0 place-items-center rounded-[var(--radius-sm)] border border-[var(--border)] text-[var(--text-3)] transition-colors hover:border-[var(--accent)] hover:text-[var(--accent)] lg:grid"><Plus aria-hidden="true" className="h-3.5 w-3.5" /></Link>
        <span aria-hidden="true" className="mx-0.5 hidden w-2 shrink-0 text-center font-mono text-xl leading-none text-[var(--border)] sm:block" style={{ transform: 'scaleX(0.4) scaleY(1.9)' }}>&gt;</span>
        <ProjectSwitcher projects={organizationProjects} activeProjectID={projectID} organizationID={activeOrganizationID} onSelect={(selectedProjectID) => void navigate({ to: '/projects/$projectId', params: { projectId: selectedProjectID } })} />
        {activeOrganizationID && <Link to="/projects/new" search={{ organizationId: activeOrganizationID }} aria-label="New project" title="New project" className="hidden h-8 w-8 shrink-0 place-items-center rounded-[var(--radius-sm)] border border-[var(--border)] text-[var(--text-3)] transition-colors hover:border-[var(--accent)] hover:text-[var(--accent)] lg:grid"><Plus aria-hidden="true" className="h-3.5 w-3.5" /></Link>}

        <nav aria-label="Primary navigation" className="absolute inset-x-0 bottom-0 grid h-11 grid-cols-5 border-t border-[var(--border-soft)] bg-[var(--panel)] md:hidden">
          {navItems.map((item) => { const active = item.href === '/' ? location.pathname === '/' || location.pathname.startsWith('/projects/') : location.pathname === item.href; const Icon = item.icon; return <Link key={item.href} to={item.href} className={`relative flex items-center justify-center gap-1.5 px-2 py-2 text-[10px] font-medium transition-colors ${active ? 'text-[var(--text)] after:absolute after:inset-x-2 after:bottom-0 after:h-0.5 after:bg-[var(--accent)]' : 'text-[var(--text-3)] hover:text-[var(--text)]'}`}><Icon aria-hidden="true" className={`h-3.5 w-3.5 ${active ? 'text-[var(--accent)]' : ''}`} /><span className="hidden sm:inline">{item.label}</span></Link> })}
        </nav>

        <button type="button" onClick={onOpenSearch} aria-label="Open search" className="group ml-auto grid h-9 w-9 shrink-0 place-items-center rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--card)] text-[var(--text-2)] transition-colors hover:border-[var(--accent)] hover:text-[var(--accent)] md:hidden"><Search aria-hidden="true" className="h-4 w-4" /></button>
        <button type="button" onClick={onOpenSearch} className="group relative ml-auto hidden h-10 shrink-0 items-center overflow-hidden rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--bg)] text-left shadow-[inset_0_1px_rgba(255,255,255,0.025)] transition-all hover:border-[var(--text-3)] hover:bg-[var(--card)] md:flex md:w-40 lg:w-56 xl:w-72"><span className="grid h-full w-10 shrink-0 place-items-center border-r border-[var(--border)] bg-[var(--card)] text-[var(--text-3)] transition-colors group-hover:text-[var(--accent)]"><Search aria-hidden="true" className="h-3.5 w-3.5" /></span><span className="min-w-0 flex-1 truncate px-3 text-xs text-[var(--text-3)] group-hover:text-[var(--text-2)]"><span className="lg:hidden">Search...</span><span className="hidden lg:inline">Search projects and commands</span></span><kbd className="mr-2.5 shrink-0 rounded-[4px] border border-[var(--border)] bg-[var(--panel)] px-1.5 py-1 font-mono text-[9px] leading-none text-[var(--text-3)]">⌘K</kbd><span className="absolute inset-x-0 bottom-0 h-px origin-left scale-x-0 bg-[var(--accent)] transition-transform duration-200 group-hover:scale-x-100" /></button>
        {projectID && <span className="hidden xl:inline-flex"><HealthPill {...health} /></span>}
        <UserMenu session={session} loggingOut={loggingOut} onLogout={onLogout} />
      </div>
    </header>
    <aside className="fixed inset-y-0 left-0 z-30 hidden w-16 border-r border-[var(--border)] bg-[var(--panel)] pt-16 md:flex md:flex-col">
      <nav aria-label="Primary navigation" className="flex flex-1 flex-col items-center gap-1 px-2 py-4">
        {navItems.map((item) => {
          const active = item.href === '/' ? location.pathname === '/' || location.pathname.startsWith('/projects/') : location.pathname === item.href
          const Icon = item.icon
          return <Link key={item.href} to={item.href} aria-label={item.label} className={`group/nav relative grid h-11 w-11 place-items-center rounded-[var(--radius-sm)] transition-colors ${active ? 'bg-[var(--accent-soft)] text-[var(--accent)] before:absolute before:-left-2 before:h-6 before:w-0.5 before:bg-[var(--accent)]' : 'text-[var(--text-3)] hover:bg-[var(--card)] hover:text-[var(--text)]'}`}><Icon aria-hidden="true" className="h-[18px] w-[18px]" strokeWidth={1.7} /><span className="pointer-events-none absolute left-[calc(100%+12px)] top-1/2 z-50 hidden -translate-y-1/2 whitespace-nowrap rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--card)] px-2.5 py-1.5 text-xs font-medium text-[var(--text)] shadow-[0_10px_30px_rgba(0,0,0,0.4)] group-hover/nav:block group-focus-visible/nav:block">{item.label}</span></Link>
        })}
      </nav>
      {projectID && <div className="group/health relative grid h-14 place-items-center border-t border-[var(--border-soft)]"><span className="h-2 w-2 rounded-full" style={{ backgroundColor: healthColor(health.tone) }} /><span className="pointer-events-none absolute left-[calc(100%+12px)] top-1/2 z-50 hidden -translate-y-1/2 whitespace-nowrap rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--card)] px-2.5 py-1.5 text-xs text-[var(--text)] shadow-lg group-hover/health:block">Project health: {health.label}</span></div>}
    </aside>
    </>
  )
}

interface ProjectSwitcherProps {
  projects: Project[]
  activeProjectID?: string
  organizationID: string
  onSelect: (projectID: string) => void
}

function ProjectSwitcher({ projects, activeProjectID, organizationID, onSelect }: ProjectSwitcherProps) {
  const detailsRef = useRef<HTMLDetailsElement>(null)
  const activeProject = projects.find((project) => project.id === activeProjectID)

  function selectProject(projectID: string) {
    detailsRef.current?.removeAttribute('open')
    onSelect(projectID)
  }

  return <details ref={detailsRef} className="group relative shrink-0"><summary aria-label="Switch project" className="flex h-10 w-9 cursor-pointer list-none items-center gap-2 rounded-[var(--radius-sm)] px-2 outline-none transition-colors hover:bg-[var(--card)] focus-visible:ring-2 focus-visible:ring-[var(--accent)] sm:w-40 [&::-webkit-details-marker]:hidden"><span className="grid h-7 w-7 shrink-0 place-items-center rounded-[5px] border border-[var(--border)] bg-[var(--card-raised)] text-[var(--text-2)]"><Database aria-hidden="true" className="h-3.5 w-3.5" /></span><span className="hidden min-w-0 flex-1 sm:block"><span className="block truncate text-xs font-semibold">{activeProject?.name ?? 'All projects'}</span><span className="block truncate font-mono text-[9px] text-[var(--text-3)]">project</span></span><ChevronDown aria-hidden="true" className="hidden h-3.5 w-3.5 shrink-0 text-[var(--text-3)] transition-transform group-open:rotate-180 sm:block" /></summary><div className="fixed inset-x-3 top-[60px] z-50 overflow-hidden rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--card)] shadow-[0_20px_60px_rgba(0,0,0,0.55)] sm:absolute sm:inset-x-auto sm:left-0 sm:top-12 sm:w-72"><div className="flex items-center justify-between border-b border-[var(--border-soft)] px-3 py-2.5"><p className="font-mono text-[9px] uppercase tracking-[0.16em] text-[var(--text-3)]">Switch project</p><span className="font-mono text-[9px] text-[var(--text-3)]">{projects.length}</span></div><div className="max-h-72 overflow-y-auto p-1.5">{projects.map((project) => <button key={project.id} type="button" onClick={() => selectProject(project.id)} className="flex w-full items-center gap-3 rounded-[5px] px-2 py-2 text-left hover:bg-[var(--card-raised)]"><span className="grid h-8 w-8 place-items-center rounded-[5px] border border-[var(--border)] bg-[var(--panel)] text-[var(--text-2)]"><Database aria-hidden="true" className="h-3.5 w-3.5" /></span><span className="min-w-0 flex-1"><span className="block truncate text-xs font-medium">{project.name}</span><span className="block truncate font-mono text-[9px] text-[var(--text-3)]">{project.id}</span></span>{project.id === activeProjectID && <Check aria-hidden="true" className="h-3.5 w-3.5 text-[var(--accent)]" />}</button>)}{projects.length === 0 && <p className="px-3 py-6 text-center text-xs text-[var(--text-3)]">No projects in this organization</p>}</div><Link to="/" search={{ organizationId: organizationID || undefined }} onClick={() => detailsRef.current?.removeAttribute('open')} className="flex items-center gap-2 border-t border-[var(--border-soft)] px-3 py-3 text-xs font-medium text-[var(--text-2)] hover:bg-[var(--card-raised)] hover:text-[var(--text)]"><LayoutGrid aria-hidden="true" className="h-3.5 w-3.5 text-[var(--accent)]" />View all projects</Link></div></details>
}

interface OrganizationSwitcherProps {
  organizations: Organization[]
  activeOrganizationID: string
  onSelect: (organizationID: string) => void
}

function OrganizationSwitcher({ organizations, activeOrganizationID, onSelect }: OrganizationSwitcherProps) {
  const detailsRef = useRef<HTMLDetailsElement>(null)
  const activeOrganization = organizations.find((organization) => organization.id === activeOrganizationID)

  function selectOrganization(organizationID: string) {
    detailsRef.current?.removeAttribute('open')
    onSelect(organizationID)
  }

  return <details ref={detailsRef} className="group relative min-w-0 max-w-52 flex-1 sm:flex-none"><summary className="flex h-10 cursor-pointer list-none items-center gap-2 rounded-[var(--radius-sm)] px-2 outline-none transition-colors hover:bg-[var(--card)] focus-visible:ring-2 focus-visible:ring-[var(--accent)] [&::-webkit-details-marker]:hidden"><span className="grid h-7 w-7 shrink-0 place-items-center rounded-[5px] border border-[var(--border)] bg-[var(--card-raised)] font-mono text-[10px] font-semibold uppercase text-[var(--accent)]">{activeOrganization?.name.slice(0, 2) ?? '--'}</span><span className="min-w-0 flex-1"><span className="block truncate text-xs font-semibold">{activeOrganization?.name ?? 'No organization'}</span><span className="hidden truncate font-mono text-[9px] text-[var(--text-3)] sm:block">workspace</span></span><ChevronDown aria-hidden="true" className="h-3.5 w-3.5 shrink-0 text-[var(--text-3)] transition-transform group-open:rotate-180" /></summary><div className="fixed inset-x-3 top-[60px] z-50 overflow-hidden rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--card)] shadow-[0_20px_60px_rgba(0,0,0,0.55)] sm:absolute sm:inset-x-auto sm:left-0 sm:top-12 sm:w-72"><div className="border-b border-[var(--border-soft)] px-3 py-2.5"><p className="font-mono text-[9px] uppercase tracking-[0.16em] text-[var(--text-3)]">Switch workspace</p></div><div className="max-h-72 overflow-y-auto p-1.5">{organizations.map((organization) => <button key={organization.id} type="button" onClick={() => selectOrganization(organization.id)} className="flex w-full items-center gap-3 rounded-[5px] px-2 py-2 text-left hover:bg-[var(--card-raised)]"><span className="grid h-8 w-8 place-items-center rounded-[5px] border border-[var(--border)] bg-[var(--panel)] font-mono text-[10px] uppercase text-[var(--accent)]">{organization.name.slice(0, 2)}</span><span className="min-w-0 flex-1"><span className="block truncate text-xs font-medium">{organization.name}</span><span className="block truncate font-mono text-[9px] text-[var(--text-3)]">{organization.slug}</span></span>{organization.id === activeOrganizationID && <Check aria-hidden="true" className="h-3.5 w-3.5 text-[var(--accent)]" />}</button>)}</div><Link to="/organizations" className="flex items-center gap-2 border-t border-[var(--border-soft)] px-3 py-3 text-xs font-medium text-[var(--text-2)] hover:bg-[var(--card-raised)] hover:text-[var(--text)]"><Plus aria-hidden="true" className="h-3.5 w-3.5 text-[var(--accent)]" />Manage organizations</Link></div></details>
}

interface UserMenuProps {
  session: Session
  loggingOut: boolean
  onLogout: () => void
}

function UserMenu({ session, loggingOut, onLogout }: UserMenuProps) {
  return <details className="group relative shrink-0"><summary aria-label="Open profile menu" className="grid h-9 w-9 cursor-pointer list-none place-items-center rounded-full border border-[var(--border)] bg-[var(--card)] font-mono text-[10px] font-semibold ring-2 ring-transparent transition-all hover:border-[var(--accent)] hover:ring-[var(--accent-soft)] group-open:border-[var(--accent)] [&::-webkit-details-marker]:hidden">{sessionInitials(session.user_id)}</summary><div className="absolute right-0 top-12 w-64 overflow-hidden rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--card)] shadow-[0_20px_60px_rgba(0,0,0,0.55)]"><div className="border-b border-[var(--border-soft)] p-4"><div className="flex items-center gap-3"><span className="grid h-9 w-9 place-items-center rounded-full bg-[var(--accent)] font-mono text-[10px] font-bold text-[var(--accent-contrast)]">{sessionInitials(session.user_id)}</span><div className="min-w-0"><p className="text-xs font-semibold">Your account</p><p className="mt-0.5 truncate font-mono text-[10px] text-[var(--text-3)]">{session.user_id}</p></div></div></div><button type="button" disabled={loggingOut} onClick={onLogout} className="w-full px-4 py-3 text-left text-xs font-medium text-[var(--text-2)] hover:bg-[var(--card-raised)] hover:text-[var(--text)] disabled:opacity-60">{loggingOut ? 'Logging out...' : 'Log out'}</button></div></details>
}
