import { Link, createFileRoute } from '@tanstack/react-router'
import { Plus } from 'lucide-react'
import { listClusters, listOrganizations, listProjects } from '../../api'
import { ConnectHostEmptyState } from '../../components/ConnectHostEmptyState'
import { HealthPill } from '../../components/shell/TopBar'
import type { Cluster, Project } from '../../types/resources'

interface ProjectSummary {
  project: Project
  clusters: Cluster[]
  nodeCount: number
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
  return { projects: summaries, organizations }
}

export const Route = createFileRoute('/_authenticated/')({
  ssr: false,
  loader: loadProjects,
  component: ProjectsPage,
})

function ProjectsPage() {
  const { projects, organizations } = Route.useLoaderData()
  const clusterCount = projects.reduce((total, summary) => total + summary.clusters.length, 0)
  const nodeCount = projects.reduce((total, summary) => total + summary.nodeCount, 0)

  return (
    <main className="min-h-[calc(100vh-56px)] px-4 py-6 text-[var(--text)] sm:px-6 lg:px-8">
      <div className="mx-auto max-w-7xl">
        <header className="flex flex-col justify-between gap-3 sm:flex-row sm:items-center">
          <div>
            <h1 className="text-xl font-semibold">Projects</h1>
            <p className="mt-1 text-sm text-[var(--text-2)]">Postgres infrastructure registered with this control plane.</p>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <div className="flex divide-x divide-[var(--border)] rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--panel)] text-xs">
              <span className="px-3 py-2"><strong className="mr-1.5 font-mono text-[var(--text)]">{projects.length}</strong><span className="text-[var(--text-2)]">Projects</span></span>
              <span className="px-3 py-2"><strong className="mr-1.5 font-mono text-[var(--text)]">{clusterCount}</strong><span className="text-[var(--text-2)]">Clusters</span></span>
              <span className="px-3 py-2"><strong className="mr-1.5 font-mono text-[var(--text)]">{nodeCount}</strong><span className="text-[var(--text-2)]">Nodes</span></span>
            </div>
            {organizations.length > 0 ? <Link to="/projects/new" className="inline-flex items-center gap-2 rounded-[var(--radius-sm)] bg-[var(--accent)] px-3.5 py-2 text-xs font-semibold text-[var(--accent-contrast)] hover:bg-[var(--accent-hover)]"><Plus aria-hidden="true" className="h-3.5 w-3.5" />Create project</Link> : <button type="button" disabled className="inline-flex cursor-not-allowed items-center gap-2 rounded-[var(--radius-sm)] bg-[var(--accent)] px-3.5 py-2 text-xs font-semibold text-[var(--accent-contrast)] opacity-50" title="Join an organization before creating a project"><Plus aria-hidden="true" className="h-3.5 w-3.5" />Create project</button>}
          </div>
        </header>

        {projects.length > 0 ? (
          <div className="mt-5 overflow-x-auto rounded-[var(--radius-sm)] border border-[var(--border)]">
            <table className="w-full min-w-[760px] border-collapse text-left text-sm">
              <thead className="bg-[var(--panel)] text-xs font-medium text-[var(--text-2)]"><tr><th className="px-4 py-3">Project</th><th className="px-4 py-3">Status</th><th className="px-4 py-3 text-right">Clusters</th><th className="px-4 py-3 text-right">Nodes</th><th className="px-4 py-3">Updated</th><th className="px-4 py-3"><span className="sr-only">Action</span></th></tr></thead>
              <tbody className="divide-y divide-[var(--border-soft)] bg-[var(--card)]">
                {projects.map(({ project, clusters, nodeCount: projectNodeCount }) => <tr key={project.id} className="hover:bg-[var(--card-raised)]">
                  <td className="px-4 py-3"><Link to="/projects/$projectId" params={{ projectId: project.id }} className="font-medium text-[var(--text)] hover:underline">{project.name}</Link><div className="mt-0.5 font-mono text-[11px] text-[var(--text-3)]">{project.id}</div></td>
                  <td className="px-4 py-3"><HealthPill label="No telemetry" tone="unknown" /></td>
                  <td className="px-4 py-3 text-right font-mono">{clusters.length}</td>
                  <td className="px-4 py-3 text-right font-mono">{projectNodeCount}</td>
                  <td className="px-4 py-3 font-mono text-xs text-[var(--text-2)]">{new Date(project.updated_at).toLocaleString()}</td>
                  <td className="px-4 py-3 text-right"><Link to="/projects/$projectId" params={{ projectId: project.id }} className="text-xs font-medium text-[var(--accent)] hover:underline">View topology</Link></td>
                </tr>)}
              </tbody>
            </table>
          </div>
        ) : (
          <ConnectHostEmptyState className="mt-5" title="No projects registered" description="Create a project, then register an Orca agent to connect the host that will run your Postgres infrastructure." />
        )}
      </div>
    </main>
  )
}
