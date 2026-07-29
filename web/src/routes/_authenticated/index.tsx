import { Link, createFileRoute } from '@tanstack/react-router'
import { motion, useReducedMotion } from 'framer-motion'
import { listClusters, listProjects } from '../../api'
import { HealthPill } from '../../components/shell/TopBar'
import { BrushHighlight } from '../../components/BrushHighlight'
import type { Cluster, Project } from '../../types/resources'

interface ProjectSummary {
  project: Project
  clusters: Cluster[]
  nodeCount: number
}

function countNodes(clusters: Cluster[]): number {
  return clusters.reduce((count, cluster) => count + 1 + cluster.replicas.length + (cluster.pgbouncer_enabled ? 1 : 0), 0)
}

async function loadProjects(): Promise<ProjectSummary[]> {
  const projects = await listProjects()
  return Promise.all(projects.map(async (project) => {
    const clusters = await listClusters(project.id)
    return { project, clusters, nodeCount: countNodes(clusters) }
  }))
}

export const Route = createFileRoute('/_authenticated/')({
  ssr: false,
  loader: loadProjects,
  component: ProjectsPage,
})

function ProjectsPage() {
  const projects = Route.useLoaderData()
  const reduceMotion = useReducedMotion()
  const clusterCount = projects.reduce((total, summary) => total + summary.clusters.length, 0)
  const nodeCount = projects.reduce((total, summary) => total + summary.nodeCount, 0)

  return (
    <main className="relative min-h-[calc(100vh-64px)] overflow-hidden px-5 py-10 font-[var(--font-sans)] text-[var(--text)] sm:px-8 sm:py-14">
      <div className="relative mx-auto max-w-6xl">
        <header className="flex flex-col justify-between gap-5 border-b border-[var(--border-soft)] pb-8 sm:flex-row sm:items-end">
          <div>
            <p className="mb-3 font-mono text-[10px] font-semibold uppercase tracking-[0.22em] text-[var(--accent)]">Infrastructure workspace</p>
            <h1 className="text-3xl font-semibold tracking-[-0.025em] sm:text-4xl">Your <BrushHighlight>projects</BrushHighlight></h1>
            <p className="mt-3 max-w-xl text-sm leading-6 text-[var(--text-2)]">A live overview of your managed Postgres environments and their connected infrastructure.</p>
          </div>
          <div className="flex items-center gap-2 text-xs text-[var(--text-3)]">
            <span className="font-mono text-[var(--text-2)]">{projects.length.toString().padStart(2, '0')}</span>
            <span>{projects.length === 1 ? 'project' : 'projects'} connected</span>
          </div>
        </header>

        {projects.length > 0 && (
          <section aria-label="Workspace totals" className="mt-6 grid grid-cols-3 overflow-hidden rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--panel)]">
            {[['Projects', projects.length], ['Clusters', clusterCount], ['Nodes', nodeCount]].map(([label, value], index) => (
              <div key={label} className={`px-4 py-4 sm:px-5 ${index > 0 ? 'border-l border-[var(--border-soft)]' : ''}`}>
                <p className="font-mono text-lg font-medium text-[var(--accent)] sm:text-xl">{value}</p>
                <p className="mt-1 text-[10px] uppercase tracking-[0.12em] text-[var(--text-3)]">{label}</p>
              </div>
            ))}
          </section>
        )}

        {projects.length > 0 ? (
          <section aria-label="Projects" className="mt-6 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {projects.map(({ project, clusters, nodeCount }, index) => (
              <motion.div
                key={project.id}
                initial={{ opacity: 0, y: reduceMotion ? 0 : 8 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ duration: reduceMotion ? 0.01 : 0.4, delay: reduceMotion ? 0 : index * 0.055, ease: [0.22, 1, 0.36, 1] }}
              >
                <Link
                  to="/projects/$projectId"
                  params={{ projectId: project.id }}
                  className="group relative block min-h-64 overflow-hidden rounded-[var(--radius-xl)] border border-[var(--border)] bg-[var(--card)] p-6 shadow-[0_16px_45px_rgba(0,0,0,0.16)] transition duration-[var(--dur-base)] ease-[var(--ease-premium)] hover:-translate-y-1 hover:border-[var(--accent)] hover:bg-[var(--card-raised)] hover:shadow-[0_24px_60px_rgba(0,0,0,0.28)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]"
                >
                  <span aria-hidden="true" className="absolute left-6 top-0 h-1.5 w-20 rounded-b-full bg-[var(--accent)]" />
                  <div className="flex items-start justify-between gap-4">
                    <div className="grid h-10 w-10 place-items-center rounded-[var(--radius-md)] border border-white/[0.07] bg-black/20 shadow-[inset_0_1px_rgba(255,255,255,0.04)]">
                      <svg aria-hidden="true" viewBox="0 0 24 24" className="h-5 w-5 fill-none stroke-[var(--accent)]" strokeWidth="1.5"><ellipse cx="12" cy="6" rx="7" ry="3"/><path d="M5 6v6c0 1.66 3.13 3 7 3s7-1.34 7-3V6M5 12v6c0 1.66 3.13 3 7 3s7-1.34 7-3v-6"/></svg>
                    </div>
                    <HealthPill label="No telemetry" tone="unknown" />
                  </div>
                  <h2 className="mt-6 min-w-0 truncate text-lg font-semibold tracking-[-0.025em] text-[var(--text)]">{project.name}</h2>
                  <p className="mt-1 font-mono text-[10px] uppercase tracking-[0.1em] text-[var(--text-3)]">{project.id.slice(0, 8)}</p>
                  <dl className="mt-6 grid grid-cols-3 gap-3 border-t border-[var(--border-soft)] pt-4">
                    <div>
                      <dt className="text-[10px] uppercase tracking-[0.12em] text-[var(--text-3)]">Clusters</dt>
                      <dd className="mt-1.5 font-mono text-sm text-[var(--text)]">{clusters.length}</dd>
                    </div>
                    <div>
                      <dt className="text-[10px] uppercase tracking-[0.12em] text-[var(--text-3)]">Nodes</dt>
                      <dd className="mt-1.5 font-mono text-sm text-[var(--text)]">{nodeCount}</dd>
                    </div>
                    <div className="text-right">
                      <dt className="text-[10px] uppercase tracking-[0.12em] text-[var(--text-3)]">Updated</dt>
                      <dd className="mt-1.5 truncate font-mono text-[11px] text-[var(--text-2)]">{new Date(project.updated_at).toLocaleDateString()}</dd>
                    </div>
                  </dl>
                  <span className="mt-5 flex items-center justify-between border-t border-[var(--border-soft)] pt-4 text-xs font-medium text-[var(--text-2)] transition-colors group-hover:text-[var(--accent)]"><span>Open topology</span><span aria-hidden="true" className="grid h-6 w-6 place-items-center rounded-full bg-[var(--accent)] text-[var(--accent-contrast)] transition-transform group-hover:translate-x-0.5">→</span></span>
                </Link>
              </motion.div>
            ))}
          </section>
        ) : (
          <section className="grid min-h-[55vh] place-items-center text-center">
            <div className="max-w-md">
              <div aria-hidden="true" className="mx-auto mb-6 grid h-14 w-14 place-items-center rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--card)] shadow-[0_20px_50px_rgba(0,0,0,0.3)]">
                <span className="h-5 w-5 rounded-full border-[5px] border-[var(--accent)]" />
              </div>
              <h2 className="text-2xl font-semibold tracking-[-0.02em]">Connect your first project</h2>
              <p className="mt-2 text-sm leading-6 text-[var(--text-2)]">Connect an Orca agent to start seeing your Postgres infrastructure.</p>
              <span className="mt-6 inline-block cursor-not-allowed" title="coming soon">
                <button type="button" disabled aria-describedby="connect-project-note" className="rounded-[var(--radius-md)] bg-[var(--accent)] px-4 py-2.5 text-sm font-semibold text-[var(--accent-contrast)] opacity-55">
                  Connect a project
                </button>
              </span>
              <span id="connect-project-note" className="sr-only">Coming soon</span>
            </div>
          </section>
        )}
      </div>
    </main>
  )
}
