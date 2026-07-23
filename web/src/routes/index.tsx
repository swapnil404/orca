import { Link, createFileRoute } from '@tanstack/react-router'
import { ApiError, listProjects } from '../api'

export const Route = createFileRoute('/')({
  ssr: false,
  loader: () => listProjects(),
  component: ProjectsPage,
  errorComponent: ProjectsError,
})

function ProjectsPage() {
  const projects = Route.useLoaderData()
  return (
    <main className="min-h-screen bg-[#07110f] px-5 py-12 text-slate-100 sm:px-10">
      <div className="mx-auto max-w-5xl">
        <p className="mb-3 text-xs font-bold uppercase tracking-[0.25em] text-emerald-300">Orca control plane</p>
        <h1 className="text-4xl font-semibold tracking-tight sm:text-6xl">Infrastructure, as reported.</h1>
        <p className="mt-5 max-w-2xl text-base leading-7 text-slate-400">Read-only project topology backed by the server's persisted desired state and live agent snapshots.</p>
        <section className="mt-12 grid gap-4 sm:grid-cols-2">
          {projects.map((project) => (
            <Link key={project.id} to="/projects/$projectId" params={{ projectId: project.id }} className="group rounded-2xl border border-white/10 bg-white/[0.03] p-5 transition hover:border-emerald-300/30 hover:bg-emerald-300/[0.04]">
              <h2 className="text-lg font-semibold text-white">{project.name}</h2>
              <p className="mt-2 text-xs text-slate-500">Updated {new Date(project.updated_at).toLocaleString()}</p>
              <p className="mt-6 text-sm font-medium text-emerald-300">Open topology</p>
            </Link>
          ))}
        </section>
        {projects.length === 0 && <p className="mt-12 rounded-2xl border border-dashed border-white/15 p-8 text-center text-slate-400">No projects are available.</p>}
      </div>
    </main>
  )
}

interface ProjectsErrorProps {
  error: Error
}

function ProjectsError({ error }: ProjectsErrorProps) {
  const authenticationMissing = error instanceof ApiError && error.status === 401
  return (
    <main className="grid min-h-screen place-items-center bg-[#07110f] p-6 text-slate-100">
      <section className="max-w-lg rounded-3xl border border-white/10 bg-white/[0.03] p-8">
        <p className="text-xs font-bold uppercase tracking-[0.2em] text-amber-300">Unable to load projects</p>
        <h1 className="mt-3 text-3xl font-semibold">{authenticationMissing ? 'Authentication is not wired on this server.' : 'The API request failed.'}</h1>
        <p className="mt-4 leading-7 text-slate-400">{authenticationMissing ? 'This checkout has no email/password JWT endpoint, so the frontend cannot provide a real login flow without inventing a server route.' : error.message}</p>
      </section>
    </main>
  )
}
