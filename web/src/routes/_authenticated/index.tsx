import { Link, createFileRoute } from '@tanstack/react-router'
import { ArrowUpRight, Boxes, Clock3, Database, Network, Plus, Server } from 'lucide-react'
import { listClusters } from '../../api'
import { ConnectHostEmptyState } from '../../components/ConnectHostEmptyState'
import type { Organization } from '../../types/organizations'
import type { Cluster, Project } from '../../types/resources'

interface ProjectSummary {
  project: Project
  clusters: Cluster[]
  nodeCount: number
}

interface OrganizationProjects {
  organization: Organization
  projects: ProjectSummary[]
}

interface ProjectsSearch {
  organizationId?: string
}

function countNodes(clusters: Cluster[]): number {
  return clusters.reduce((count, cluster) => count + 1 + cluster.replicas.length + (cluster.pgbouncer_enabled ? 1 : 0), 0)
}

async function loadProjects(projects: Project[], organizations: Organization[]) {
  const summaries = await Promise.all(projects.map(async (project) => {
    const clusters = await listClusters(project.id)
    return { project, clusters, nodeCount: countNodes(clusters) }
  }))
  const groups: OrganizationProjects[] = organizations.map((organization) => ({
    organization,
    projects: summaries.filter(({ project }) => project.organization_id === organization.id),
  }))
  return { groups, organizations }
}

export const Route = createFileRoute('/_authenticated/')({
  ssr: false,
  validateSearch: (search: Record<string, unknown>): ProjectsSearch => ({
    organizationId: typeof search.organizationId === 'string' ? search.organizationId : undefined,
  }),
  loader: ({ context }) => loadProjects(context.projects, context.organizations),
  component: ProjectsPage,
})

function ProjectsPage() {
  const { groups, organizations } = Route.useLoaderData()
  const search = Route.useSearch()
  const activeOrganizationID = organizations.some((organization) => organization.id === search.organizationId)
    ? search.organizationId
    : organizations[0]?.id
  const visibleGroups = groups.filter(({ organization }) => organization.id === activeOrganizationID)
  const visibleProjects = visibleGroups.flatMap((group) => group.projects)
  const clusterCount = visibleProjects.reduce((total, summary) => total + summary.clusters.length, 0)
  const nodeCount = visibleProjects.reduce((total, summary) => total + summary.nodeCount, 0)
  const selectedOrganization = organizations.find((organization) => organization.id === activeOrganizationID)

  return (
    <main className="relative min-h-[calc(100vh-64px)] overflow-hidden px-4 py-8 text-[var(--text)] sm:px-6 lg:px-10 lg:py-12">
      <div aria-hidden="true" className="pointer-events-none absolute -right-48 -top-64 h-[560px] w-[560px] rounded-full bg-[var(--accent)] opacity-[0.035] blur-3xl" />
      <div className="relative mx-auto max-w-7xl">
        <header className="grid gap-7 border-b border-[var(--border)] pb-8 lg:grid-cols-[minmax(0,1fr)_360px] lg:items-end">
          <div>
            <p className="mb-3 font-mono text-[10px] font-medium uppercase tracking-[0.18em] text-[var(--accent)]">Infrastructure registry</p>
            <h1 className="max-w-2xl text-3xl font-semibold tracking-[-0.045em] sm:text-4xl">
              {selectedOrganization ? <>Projects in <span className="text-[var(--accent)]">{selectedOrganization.name}</span></> : <>Your Postgres <span className="text-[var(--accent)]">projects</span></>}
            </h1>
            <p className="mt-3 max-w-xl text-sm leading-6 text-[var(--text-2)]">Projects are isolated by organization and reconciled continuously against infrastructure running on your hosts.</p>
          </div>
          <section aria-label="Project overview" className="overflow-hidden rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--panel)] shadow-[0_16px_45px_rgba(0,0,0,0.16)]">
            <div className="flex items-center justify-between border-b border-[var(--border-soft)] px-3.5 py-2">
              <span className="font-mono text-[9px] uppercase tracking-[0.16em] text-[var(--text-3)]">Current scope</span>
              <span className="inline-flex items-center gap-1.5 font-mono text-[9px] text-[var(--text-3)]"><span className="h-1.5 w-1.5 rounded-full bg-[var(--accent)]" />Indexed</span>
            </div>
            <div className="grid grid-cols-3 divide-x divide-[var(--border-soft)]">
              <Metric icon={Boxes} value={visibleProjects.length} label="Projects" />
              <Metric icon={Database} value={clusterCount} label="Clusters" />
              <Metric icon={Network} value={nodeCount} label="Nodes" />
            </div>
          </section>
        </header>

        {groups.length > 0 ? (
          <div className="mt-8 space-y-9">
            {visibleGroups.map(({ organization, projects: organizationProjects }) => (
              <section key={organization.id} aria-labelledby={`organization-${organization.id}`}>
                <header className="mb-4 flex items-end justify-between gap-4">
                  <div className="flex items-center gap-3">
                    <span className="grid h-9 w-9 place-items-center rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--card)] font-mono text-[10px] font-semibold uppercase text-[var(--accent)]">{organization.name.slice(0, 2)}</span>
                    <div>
                      <h2 id={`organization-${organization.id}`} className="text-sm font-semibold">{organization.name}</h2>
                      <p className="mt-0.5 font-mono text-[10px] text-[var(--text-3)]">{organization.slug} / {organizationProjects.length} {organizationProjects.length === 1 ? 'project' : 'projects'}</p>
                    </div>
                  </div>
                  <Link to="/projects/new" search={{ organizationId: organization.id }} className="group inline-flex items-center gap-2 rounded-[var(--radius-sm)] bg-[var(--accent)] px-3.5 py-2 text-xs font-semibold text-[var(--accent-contrast)] transition-colors hover:bg-[var(--accent-hover)]"><Plus aria-hidden="true" className="h-3.5 w-3.5" />New project</Link>
                </header>
                {organizationProjects.length > 0 ? <ProjectGrid projects={organizationProjects} /> : <EmptyOrganization />}
              </section>
            ))}
          </div>
        ) : (
          <ConnectHostEmptyState className="mt-5" title="No organizations available" description="Create or join an organization before creating a project." />
        )}
      </div>
    </main>
  )
}

function Metric({ icon: Icon, value, label }: { icon: typeof Boxes; value: number; label: string }) {
  return (
    <div className="group relative flex min-w-0 items-center gap-2.5 px-3 py-3.5 transition-colors hover:bg-[var(--card)]">
      <Icon aria-hidden="true" className="h-3.5 w-3.5 shrink-0 text-[var(--accent)]" strokeWidth={1.7} />
      <div className="min-w-0"><strong className="block font-mono text-lg font-medium leading-none text-[var(--text)]">{value}</strong><span className="mt-1 block truncate text-[9px] uppercase tracking-[0.1em] text-[var(--text-3)]">{label}</span></div>
      <span className="absolute inset-x-0 bottom-0 h-px origin-left scale-x-0 bg-[var(--accent)] transition-transform duration-200 group-hover:scale-x-100" />
    </div>
  )
}

function ProjectGrid({ projects }: { projects: ProjectSummary[] }) {
  return (
    <div className="grid gap-2.5 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
      {projects.map(({ project, clusters, nodeCount }) => (
        <Link key={project.id} to="/projects/$projectId" params={{ projectId: project.id }} className="group relative min-w-0 overflow-hidden rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--bg)] px-4 py-3.5 transition-all duration-200 hover:-translate-y-px hover:border-[var(--accent)]/45 hover:shadow-[0_12px_30px_rgba(0,0,0,0.2)]">
          <div aria-hidden="true" className="pointer-events-none absolute -right-12 -top-16 h-32 w-32 rounded-full bg-[var(--accent)] opacity-0 blur-3xl transition-opacity group-hover:opacity-[0.08]" />
          <div className="relative flex items-center justify-between gap-3">
            <span className="inline-flex items-center gap-2 font-mono text-[9px] uppercase tracking-[0.12em] text-[var(--text-3)]"><span className="h-1.5 w-1.5 rounded-full bg-[var(--accent)]" />Project</span>
            <ArrowUpRight aria-hidden="true" className="h-3.5 w-3.5 text-[var(--text-3)] transition-all group-hover:-translate-y-0.5 group-hover:translate-x-0.5 group-hover:text-[var(--accent)]" />
          </div>
          <h3 className="relative mt-3 truncate text-base font-semibold tracking-[-0.02em]">{project.name}</h3>
          <p className="relative mt-1 truncate font-mono text-[9px] text-[var(--text-3)]">{project.id}</p>
          <div className="relative mt-3 flex items-center gap-2 border-t border-[var(--border-soft)] pt-3">
            <ProjectStat value={clusters.length} label={clusters.length === 1 ? 'cluster' : 'clusters'} />
            <span className="h-3 w-px bg-[var(--border)]" />
            <ProjectStat value={nodeCount} label={nodeCount === 1 ? 'node' : 'nodes'} />
            <span className="ml-auto inline-flex items-center gap-1.5 text-[9px] text-[var(--text-3)]"><Clock3 aria-hidden="true" className="h-3 w-3" />{new Date(project.updated_at).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}</span>
          </div>
        </Link>
      ))}
    </div>
  )
}

function ProjectStat({ value, label }: { value: number; label: string }) {
  return <span className="inline-flex items-baseline gap-1"><strong className="font-mono text-xs font-medium text-[var(--text)]">{value}</strong><span className="text-[9px] text-[var(--text-3)]">{label}</span></span>
}

function EmptyOrganization() {
  return <div className="rounded-[var(--radius-md)] border border-dashed border-[var(--border)] bg-[var(--bg)] px-6 py-8 text-center"><Server aria-hidden="true" className="mx-auto h-5 w-5 text-[var(--text-3)]" /><p className="mt-3 text-sm font-medium">No infrastructure here yet</p><p className="mt-1 text-xs text-[var(--text-3)]">Create the first project in this organization.</p></div>
}
