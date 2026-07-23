import { Link, createFileRoute } from '@tanstack/react-router'
import { getProjectTopology } from '../../api'
import { CanvasView } from '../../canvas/CanvasView'
import { useProjectEvents } from '../../hooks/useProjectEvents'
import { useTopologyStore } from '../../store/topology'

export const Route = createFileRoute('/projects/$projectId')({
  ssr: false,
  loader: ({ params }) => getProjectTopology(params.projectId),
  component: ProjectCanvasPage,
})

function ProjectCanvasPage() {
  const { project, clusters } = Route.useLoaderData()
  const { projectId } = Route.useParams()
  const snapshot = useTopologyStore((state) => state.snapshot)
  const connected = useTopologyStore((state) => state.connected)
  useProjectEvents(projectId)

  return (
    <main className="flex min-h-screen flex-col bg-[#07110f] p-3 text-slate-100 sm:p-5">
      <header className="mb-4 flex flex-wrap items-end justify-between gap-4 px-2 py-3">
        <div>
          <Link to="/" className="text-xs font-bold uppercase tracking-[0.22em] text-emerald-300">Orca / Projects</Link>
          <h1 className="mt-2 text-2xl font-semibold sm:text-3xl">{project.name}</h1>
        </div>
        <div className="flex items-center gap-2 rounded-full border border-white/10 bg-white/[0.03] px-3 py-2 text-xs text-slate-400">
          <span className={`h-2 w-2 rounded-full ${connected ? 'bg-emerald-300' : 'bg-slate-500'}`} />
          {connected ? 'Live snapshot connected' : 'Actual state unavailable'}
        </div>
      </header>
      <CanvasView clusters={clusters} snapshot={snapshot} />
    </main>
  )
}
