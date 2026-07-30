import { Link, createFileRoute } from '@tanstack/react-router'
import { Settings2 } from 'lucide-react'
import { getProjectTopology } from '../../../api'
import { CanvasView } from '../../../canvas/CanvasView'
import { useProjectEvents } from '../../../hooks/useProjectEvents'
import { useTopologyStore } from '../../../store/topology'

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
    <main className="flex min-h-[calc(100vh-56px)] flex-col p-3 text-[var(--text)] sm:p-5">
      <header className="mb-3 flex flex-wrap items-center justify-between gap-3 px-1 py-1">
        <div>
          <p className="font-mono text-[10px] font-medium uppercase tracking-[0.16em] text-[var(--text-3)]">
            <Link to="/organizations" className="hover:text-[var(--text)]">Orca</Link>
            <span className="mx-2 text-[var(--border)]">/</span>
            <span className="text-[var(--text-2)]">{project.name}</span>
          </p>
          <h1 className="mt-1.5 text-xl font-semibold">Project topology</h1>
        </div>
        <div className="flex items-center gap-2">
          <Link to="/projects/$projectId/settings" params={{ projectId }} className="inline-flex items-center gap-2 rounded-full border border-[var(--border)] bg-[var(--panel)] px-3.5 py-2 text-[11px] font-medium text-[var(--text-2)] hover:border-[var(--text-3)] hover:text-[var(--text)]"><Settings2 className="h-3.5 w-3.5" />Settings</Link>
          <div className="flex items-center gap-2.5 rounded-full border border-[var(--border)] bg-[var(--panel)] px-3.5 py-2 text-[11px] font-medium text-[var(--text-2)] shadow-[inset_0_1px_rgba(255,255,255,0.03)]">
            <span className={`h-1.5 w-1.5 rounded-full ${connected ? 'bg-[var(--healthy)]' : 'bg-[var(--text-3)]'}`} />
            {connected ? 'Live telemetry' : 'Telemetry unavailable'}
          </div>
        </div>
      </header>
      <CanvasView clusters={clusters} snapshot={snapshot} />
    </main>
  )
}
