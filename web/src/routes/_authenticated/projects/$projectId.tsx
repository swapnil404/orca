import { Link, createFileRoute } from '@tanstack/react-router'
import { Copy, Settings2, Terminal } from 'lucide-react'
import { useEffect, useState } from 'react'
import { getProjectTopology, listProjectHosts, rotateHostToken } from '../../../api'
import { CanvasView } from '../../../canvas/CanvasView'
import { ConnectHostEmptyState } from '../../../components/ConnectHostEmptyState'
import { useProjectEvents } from '../../../hooks/useProjectEvents'
import { useTopologyStore } from '../../../store/topology'

export const Route = createFileRoute('/_authenticated/projects/$projectId')({
  ssr: false,
  loader: async ({ params }) => {
    const [topology, hosts] = await Promise.all([getProjectTopology(params.projectId), listProjectHosts(params.projectId)])
    return { ...topology, hosts }
  },
  component: ProjectCanvasPage,
})

function ProjectCanvasPage() {
  const initialTopology = Route.useLoaderData()
  const { project } = initialTopology
  const [clusters, setClusters] = useState(initialTopology.clusters)
  const { projectId } = Route.useParams()
  const [hostState, setHostState] = useState({ projectID: projectId, hosts: initialTopology.hosts })
  const snapshot = useTopologyStore((state) => state.snapshot)
  const connected = useTopologyStore((state) => state.connected)
  useProjectEvents(projectId)
  useEffect(() => setClusters(initialTopology.clusters), [initialTopology.clusters, projectId])
  const projectSnapshot = snapshot?.project_id === projectId ? snapshot : null
  const hosts = hostState.projectID === projectId ? hostState.hosts : initialTopology.hosts
  const awaitingHost = hosts.find((host) => host.status === 'never_connected')
  const [commandState, setCommandState] = useState<{ hostID: string; command: string } | null>(() => {
    const host = initialTopology.hosts.find((item) => item.status === 'never_connected')
    const command = host && typeof window !== 'undefined' ? window.sessionStorage.getItem(`orca.host-command.${host.id}`) : null
    return host && command ? { hostID: host.id, command } : null
  })
  const hostCommand = commandState && commandState.hostID === awaitingHost?.id ? commandState.command : null
  useEffect(() => setHostState({ projectID: projectId, hosts: initialTopology.hosts }), [initialTopology.hosts, projectId])
  useEffect(() => {
    let active = true
    const refresh = () => void listProjectHosts(projectId).then((nextHosts) => {
      if (active) setHostState({ projectID: projectId, hosts: nextHosts })
    }).catch(() => undefined)
    const timer = window.setInterval(refresh, 10_000)
    return () => { active = false; window.clearInterval(timer) }
  }, [projectId])
  useEffect(() => {
    if (commandState && commandState.hostID !== awaitingHost?.id) {
      const previousHost = hosts.find((host) => host.id === commandState.hostID)
      if (previousHost && previousHost.status !== 'never_connected') window.sessionStorage.removeItem(`orca.host-command.${commandState.hostID}`)
      setCommandState(null)
    }
    if (awaitingHost && commandState?.hostID !== awaitingHost.id) {
      const command = window.sessionStorage.getItem(`orca.host-command.${awaitingHost.id}`)
      if (command) setCommandState({ hostID: awaitingHost.id, command })
    }
  }, [awaitingHost, commandState])

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
      {awaitingHost && <HostConnection hostID={awaitingHost.id} command={hostCommand} onCommand={(command) => { window.sessionStorage.setItem(`orca.host-command.${awaitingHost.id}`, command); setCommandState({ hostID: awaitingHost.id, command }) }} />}
      {clusters.length === 0 ? (
        <ConnectHostEmptyState className="flex-1" title="Connect a host to get started" description="Register an Orca agent on the infrastructure that will run PostgreSQL. Once the host connects, you can configure this project's topology." />
      ) : (
        <CanvasView key={projectId} clusters={clusters} snapshot={projectSnapshot} onClusterUpdated={(updated) => setClusters((current) => current.map((cluster) => cluster.id === updated.id ? updated : cluster))} />
      )}
    </main>
  )
}

function HostConnection({ hostID, command, onCommand }: { hostID: string; command: string | null; onCommand: (command: string) => void }) {
  const [copied, setCopied] = useState(false)
  const [generating, setGenerating] = useState(false)
  const [error, setError] = useState('')
  async function copyCommand() {
    if (!command) return
    await navigator.clipboard.writeText(command)
    setCopied(true)
    window.setTimeout(() => setCopied(false), 2_000)
  }
  async function generateCommand() {
    setGenerating(true)
    setError('')
    try {
      const registration = await rotateHostToken(hostID)
      onCommand(registration.docker_run_command)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not generate a connection command.')
    } finally {
      setGenerating(false)
    }
  }
  return <section className="mb-3 rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--card)] p-4"><div className="flex flex-col gap-4 lg:flex-row lg:items-center"><div className="flex min-w-0 flex-1 gap-3"><span className="grid h-9 w-9 shrink-0 place-items-center rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--panel)] text-[var(--accent)]"><Terminal className="h-4 w-4" /></span><div><h2 className="text-sm font-semibold">Connect the registered host</h2><p className="mt-1 text-xs leading-5 text-[var(--text-2)]">Run this one-time command on the host that will provide storage and compute. Desired topology and extensions are waiting for its agent.</p></div></div>{command ? <button type="button" onClick={copyCommand} className="inline-flex shrink-0 items-center justify-center gap-2 rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--panel)] px-3.5 py-2 text-xs font-medium text-[var(--text-2)] hover:border-[var(--accent)] hover:text-[var(--text)]"><Copy className="h-3.5 w-3.5" />{copied ? 'Copied' : 'Copy command'}</button> : <button type="button" disabled={generating} onClick={generateCommand} className="inline-flex shrink-0 items-center justify-center rounded-[var(--radius-sm)] bg-[var(--accent)] px-3.5 py-2 text-xs font-semibold text-[var(--accent-contrast)] hover:bg-[var(--accent-hover)] disabled:opacity-50">{generating ? 'Generating...' : 'Generate command'}</button>}</div>{error && <p role="alert" className="mt-3 text-xs text-[var(--critical)]">{error}</p>}{command && <><pre className="mt-3 overflow-x-auto rounded-[var(--radius-sm)] bg-[var(--panel)] p-3 font-mono text-[11px] leading-5 text-[var(--text-2)]"><code>{command}</code></pre><button type="button" disabled={generating} onClick={generateCommand} className="mt-2 text-[11px] text-[var(--text-3)] hover:text-[var(--text-2)]">{generating ? 'Regenerating...' : 'Command expired? Generate a new one'}</button></>}</section>
}
