import { Link, createFileRoute } from '@tanstack/react-router'
import { getProjectTopology } from '../../../api'
import { CanvasView } from '../../../canvas/CanvasView'
import { useProjectEvents } from '../../../hooks/useProjectEvents'
import { useTopologyStore } from '../../../store/topology'
import { BrushHighlight } from '../../../components/BrushHighlight'

export const Route = createFileRoute('/_authenticated/projects/$projectId')({
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
    <main className="flex min-h-[calc(100vh-64px)] flex-col p-3 text-[var(--text)] sm:min-h-screen sm:p-5 lg:p-6">
      <header className="mb-4 flex flex-wrap items-end justify-between gap-4 px-1 py-2 sm:px-2">
        <div>
          <Link to="/" className="group inline-flex items-center gap-2 font-mono text-[10px] font-semibold uppercase tracking-[0.2em] text-[var(--text-3)] transition-colors hover:text-[var(--accent)]"><span className="transition-transform group-hover:-translate-x-0.5">←</span> Projects</Link>
          <h1 className="mt-2 text-2xl font-semibold tracking-[-0.02em] sm:text-3xl"><BrushHighlight>{project.name}</BrushHighlight></h1>
        </div>
        <div className="flex items-center gap-2.5 rounded-full border border-[var(--border)] bg-[var(--panel)] px-3.5 py-2 text-[11px] font-medium text-[var(--text-2)] shadow-[inset_0_1px_rgba(255,255,255,0.03)]">
          <span className={`relative h-1.5 w-1.5 rounded-full ${connected ? 'signal-dot bg-[var(--healthy)]' : 'bg-[var(--text-3)]'}`} />
          {connected ? 'Live telemetry' : 'Telemetry unavailable'}
        </div>
      </header>
      <CanvasView clusters={clusters} snapshot={snapshot} />
    </main>
  )
}
