import { Link, createFileRoute, useRouter } from '@tanstack/react-router'
import { Plus, X } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { ApiError, createProject, listClusters, listOrganizations, listProjects } from '../../api'
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
  const router = useRouter()
  const [creating, setCreating] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')
  const clusterCount = projects.reduce((total, summary) => total + summary.clusters.length, 0)
  const nodeCount = projects.reduce((total, summary) => total + summary.nodeCount, 0)

  async function handleCreate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = event.currentTarget
    const data = new FormData(form)
    setSubmitting(true)
    setError('')
    try {
      await createProject(String(data.get('name')), String(data.get('organization_id')))
      form.reset()
      setCreating(false)
      await router.invalidate()
    } catch (cause) {
      setError(cause instanceof ApiError ? cause.message : 'Could not create the project.')
    } finally {
      setSubmitting(false)
    }
  }

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
            <button type="button" disabled={organizations.length === 0} onClick={() => { setCreating((open) => !open); setError('') }} className="inline-flex items-center gap-2 rounded-[var(--radius-sm)] bg-[var(--accent)] px-3.5 py-2 text-xs font-semibold text-[var(--accent-contrast)] hover:bg-[var(--accent-hover)] disabled:cursor-not-allowed disabled:opacity-50" title={organizations.length === 0 ? 'Join an organization before creating a project' : undefined}>
              {creating ? <X aria-hidden="true" className="h-3.5 w-3.5" /> : <Plus aria-hidden="true" className="h-3.5 w-3.5" />}
              {creating ? 'Cancel' : 'Create project'}
            </button>
          </div>
        </header>

        {creating && (
          <form onSubmit={handleCreate} className="mt-5 rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--card)] p-4 sm:p-5">
            <div className="grid gap-4 sm:grid-cols-[minmax(0,1fr)_minmax(220px,0.55fr)_auto] sm:items-end">
              <label className="text-xs font-medium text-[var(--text-2)]">Project name<input autoFocus required name="name" maxLength={100} placeholder="Production database" className="mt-2 w-full rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--panel)] px-3 py-2.5 text-sm text-[var(--text)] outline-none placeholder:text-[var(--text-3)] focus:border-[var(--accent)] focus:ring-2 focus:ring-[var(--accent-soft)]" /></label>
              <label className="text-xs font-medium text-[var(--text-2)]">Organization<select required name="organization_id" defaultValue={organizations[0]?.id} className="mt-2 w-full rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--panel)] px-3 py-2.5 text-sm text-[var(--text)] outline-none focus:border-[var(--accent)] focus:ring-2 focus:ring-[var(--accent-soft)]">{organizations.map((organization) => <option key={organization.id} value={organization.id}>{organization.name}</option>)}</select></label>
              <button disabled={submitting} className="rounded-[var(--radius-sm)] bg-[var(--accent)] px-4 py-2.5 text-sm font-semibold text-[var(--accent-contrast)] hover:bg-[var(--accent-hover)] disabled:cursor-not-allowed disabled:opacity-50">{submitting ? 'Creating...' : 'Create'}</button>
            </div>
            {error && <p role="alert" className="mt-3 text-xs text-[var(--critical)]">{error}</p>}
          </form>
        )}

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
          <section className="mt-5 rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--card)] p-8"><h2 className="text-base font-semibold">No projects registered</h2><p className="mt-2 text-sm text-[var(--text-2)]">Register an Orca agent to report Postgres infrastructure to this control plane.</p></section>
        )}
      </div>
    </main>
  )
}
