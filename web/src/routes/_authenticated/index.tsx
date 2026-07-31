import { Link, createFileRoute } from '@tanstack/react-router'
import { ArrowUpRight, Boxes, Clock3, Database, Network, Plus, Server } from 'lucide-react'
import { listClusters, listOrganizations, listProjects } from '../../api'
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

async function loadProjects() {
  const [projects, organizations] = await Promise.all([listProjects(), listOrganizations()])
  const summaries = await Promise.all(projects.map(async (project) => {
    const clusters = await listClusters(project.id)
    return { project, clusters, nodeCount: countNodes(clusters) }
  }))
  const groups: OrganizationProjects[] = organizations.map((organization) => ({
    organization,
    projects: summaries.filter(({ project }) => project.organization_id === organization.id),
  }))
  return { projects: summaries, groups, organizations }
}

export const Route = createFileRoute('/_authenticated/')({
  ssr: false,
  validateSearch: (search: Record<string, unknown>): ProjectsSearch => ({
    organizationId: typeof search.organizationId === 'string' ? search.organizationId : undefined,
  }),
  loader: loadProjects,
  component: ProjectsPage,
})

function ProjectsPage() {
  const { groups, organizations } = Route.useLoaderData()
  const search = Route.useSearch()
  const visibleGroups = groups.filter(({ organization }) => !search.organizationId || organization.id === search.organizationId)
  const visibleProjects = visibleGroups.flatMap((group) => group.projects)
  const clusterCount = visibleProjects.reduce((total, summary) => total + summary.clusters.length, 0)
  const nodeCount = visibleProjects.reduce((total, summary) => total + summary.nodeCount, 0)
  const selectedOrganization = organizations.find((organization) => organization.id === search.organizationId)

  return (
    <main className="relative min-h-[calc(100vh-64px)] overflow-hidden px-4 py-8 text-[var(--text)] sm:px-6 lg:px-10 lg:py-12">
      <div aria-hidden="true" className="pointer-events-none absolute -right-48 -top-64 h-[560px] w-[560px] rounded-full bg-[var(--accent)] opacity-[0.035] blur-3xl" />
      <div className="relative mx-auto max-w-7xl">
        <header className="grid gap-8 border-b border-[var(--border)] pb-8 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-end">
          <div>
            <h1 className="max-w-2xl text-3xl font-semibold tracking-[-0.045em] sm:text-4xl">{selectedOrganization ? <>Projects in <span className="text-[var(--accent)]">{selectedOrganization.name}</span></> : <>Your Postgres <span className="text-[var(--accent)]">projects</span></>}</h1>
            <p className="mt-3 max-w-xl text-sm leading-6 text-[var(--text-2)]">Projects are isolated by organization and reconciled continuously against infrastructure running on your hosts.</p>
          </div>
          <div className="flex flex-wrap items-stretch gap-px overflow-hidden rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--border)]">
            <Metric icon={Boxes} value={visibleProjects.length} label="Projects" />
            <Metric icon={Database} value={clusterCount} label="Clusters" />
            <Metric icon={Network} value={nodeCount} label="Nodes" />
          </div>
        </header>

        {groups.length > 0 ? (
          <div className="mt-8 space-y-10">
            {visibleGroups.map(({ organization, projects: organizationProjects }) => (
              <section key={organization.id} aria-labelledby={`organization-${organization.id}`}>
                <header className="mb-4 flex items-end justify-between gap-4">
                  <div className="flex items-center gap-3"><span className="grid h-9 w-9 place-items-center rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--card)] font-mono text-[10px] font-semibold uppercase text-[var(--accent)]">{organization.name.slice(0, 2)}</span><div><h2 id={`organization-${organization.id}`} className="text-sm font-semibold">{organization.name}</h2><p className="mt-0.5 font-mono text-[10px] text-[var(--text-3)]">{organization.slug} / {organizationProjects.length} {organizationProjects.length === 1 ? 'project' : 'projects'}</p></div></div>
                  <Link to="/projects/new" search={{ organizationId: organization.id }} className="group inline-flex items-center gap-2 rounded-[var(--radius-sm)] bg-[var(--accent)] px-3.5 py-2 text-xs font-semibold text-[var(--accent-contrast)] transition-colors hover:bg-[var(--accent-hover)]"><Plus aria-hidden="true" className="h-3.5 w-3.5" />New project</Link>
                </header>
                {organizationProjects.length > 0 ? <ProjectGrid projects={organizationProjects} /> : <div className="rounded-[var(--radius-lg)] border border-dashed border-[var(--border)] bg-[var(--panel)] px-6 py-10 text-center"><Server aria-hidden="true" className="mx-auto h-5 w-5 text-[var(--text-3)]" /><p className="mt-3 text-sm font-medium">No infrastructure here yet</p><p className="mt-1 text-xs text-[var(--text-3)]">Create the first project in this organization.</p></div>}
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
  return <div className="flex min-w-28 items-center gap-3 bg-[var(--panel)] px-4 py-3"><Icon aria-hidden="true" className="h-4 w-4 text-[var(--accent)]" /><div><strong className="block font-mono text-base font-medium leading-none text-[var(--accent)]">{value}</strong><span className="mt-1 block text-[9px] uppercase tracking-[0.12em] text-[var(--text-3)]">{label}</span></div></div>
}

function ProjectGrid({ projects }: { projects: ProjectSummary[] }) {
  return <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">{projects.map(({ project, clusters, nodeCount }) => <Link key={project.id} to="/projects/$projectId" params={{ projectId: project.id }} className="group relative overflow-hidden rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--card)] p-5 transition-all duration-200 hover:-translate-y-0.5 hover:border-[var(--accent)]/50 hover:bg-[var(--card-raised)] hover:shadow-[0_18px_45px_rgba(0,0,0,0.24)]"><div className="absolute inset-x-0 top-0 h-px bg-gradient-to-r from-transparent via-[var(--accent)] to-transparent opacity-40 transition-opacity group-hover:opacity-90" /><div className="flex items-start justify-between gap-4"><div className="grid h-10 w-10 place-items-center rounded-[var(--radius-sm)] border border-[var(--accent)]/20 bg-[var(--accent-soft)] text-[var(--accent)]"><Database aria-hidden="true" className="h-[18px] w-[18px]" strokeWidth={1.6} /></div><ArrowUpRight aria-hidden="true" className="h-4 w-4 text-[var(--text-3)] transition-all group-hover:-translate-y-0.5 group-hover:translate-x-0.5 group-hover:text-[var(--accent)]" /></div><h3 className="mt-5 text-lg font-semibold tracking-[-0.025em]">{project.name}</h3><p className="mt-1 truncate font-mono text-[9px] uppercase tracking-[0.08em] text-[var(--text-3)]">{project.id}</p><div className="mt-5 grid grid-cols-2 border-y border-[var(--border-soft)]"><div className="py-3"><span className="block font-mono text-lg text-[var(--accent)]">{clusters.length}</span><span className="text-[10px] uppercase tracking-[0.1em] text-[var(--text-3)]">Clusters</span></div><div className="border-l border-[var(--border-soft)] py-3 pl-4"><span className="block font-mono text-lg text-[var(--accent)]">{nodeCount}</span><span className="text-[10px] uppercase tracking-[0.1em] text-[var(--text-3)]">Nodes</span></div></div><div className="mt-4 flex items-center justify-between gap-3 text-[10px] text-[var(--text-3)]"><span className="inline-flex items-center gap-1.5"><span className="h-1.5 w-1.5 rounded-full bg-[var(--accent)] opacity-60" />Telemetry pending</span><span className="inline-flex items-center gap-1.5 font-mono"><Clock3 aria-hidden="true" className="h-3 w-3" />{new Date(project.updated_at).toLocaleDateString()}</span></div></Link>)}</div>
}
