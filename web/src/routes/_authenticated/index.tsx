import { Link, createFileRoute } from '@tanstack/react-router'
import { motion, useReducedMotion } from 'framer-motion'
import { listClusters, listProjects } from '../../api'
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

  return (
    <main className="min-h-[calc(100vh-52px)] bg-[var(--bg)] px-5 py-8 font-[var(--font-sans)] text-[var(--text)] sm:px-8 sm:py-10">
      <div className="mx-auto max-w-6xl">
        <header className="flex items-baseline gap-2">
          <h1 className="text-base font-semibold tracking-[-0.01em]">Projects</h1>
          <span className="font-mono text-xs text-[var(--text-2)]">{projects.length}</span>
        </header>

        {projects.length > 0 ? (
          <section aria-label="Projects" className="mt-5 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {projects.map(({ project, nodeCount }, index) => (
              <motion.div
                key={project.id}
                initial={{ opacity: 0, y: reduceMotion ? 0 : 8 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ duration: reduceMotion ? 0.12 : 0.24, delay: reduceMotion ? 0 : index * 0.04, ease: [0.16, 1, 0.3, 1] }}
              >
                <Link
                  to="/projects/$projectId"
                  params={{ projectId: project.id }}
                  className="group block rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--card)] p-5 transition duration-[var(--dur-fast)] ease-[var(--ease-premium)] hover:-translate-y-px hover:border-[var(--text-3)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]"
                >
                  <div className="flex items-start justify-between gap-4">
                    <h2 className="min-w-0 truncate text-base font-semibold tracking-[-0.01em] text-[var(--text)]">{project.name}</h2>
                    <HealthPill label="Unknown" tone="unknown" />
                  </div>
                  <dl className="mt-8 grid grid-cols-2 gap-4 border-t border-[var(--border-soft)] pt-4">
                    <div>
                      <dt className="text-xs text-[var(--text-3)]">Nodes</dt>
                      <dd className="mt-1 font-mono text-sm text-[var(--text)]">{nodeCount}</dd>
                    </div>
                    <div className="text-right">
                      <dt className="text-xs text-[var(--text-3)]">Updated</dt>
                      <dd className="mt-1 font-mono text-xs text-[var(--text-2)]">{new Date(project.updated_at).toLocaleString()}</dd>
                    </div>
                  </dl>
                </Link>
              </motion.div>
            ))}
          </section>
        ) : (
          <section className="grid min-h-[60vh] place-items-center text-center">
            <div className="max-w-md">
              <div aria-hidden="true" className="mx-auto mb-5 grid h-11 w-11 place-items-center rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--card)]">
                <span className="h-4 w-4 rounded-[4px] bg-[var(--accent)]" />
              </div>
              <h2 className="text-xl font-semibold tracking-[-0.02em]">Connect your first project</h2>
              <p className="mt-2 text-sm leading-6 text-[var(--text-2)]">Connect an Orca agent to start seeing your Postgres infrastructure.</p>
              <span className="mt-6 inline-block cursor-not-allowed" title="coming soon">
                <button type="button" disabled aria-describedby="connect-project-note" className="rounded-[var(--radius-md)] bg-[var(--accent)] px-4 py-2.5 text-sm font-semibold text-[#111205] opacity-55">
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
