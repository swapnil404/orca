import { Link, createFileRoute } from '@tanstack/react-router'
import { Copy, Eye, EyeOff, RefreshCw, Settings2, Terminal } from 'lucide-react'
import { useEffect, useState } from 'react'
import { getProjectTopology, listProjectHosts, rotateHostToken } from '../../../api'
import { CanvasView } from '../../../canvas/CanvasView'
import { areReportsFresh } from '../../../canvas/status'
import { ConnectHostEmptyState } from '../../../components/ConnectHostEmptyState'
import { RestartProjectDialog } from '../../../components/RestartProjectDialog'
import { useProjectEvents } from '../../../hooks/useProjectEvents'
import { useRestartProject } from '../../../hooks/useRestartProject'
import { useTopologyStore } from '../../../store/topology'

export const Route = createFileRoute('/_authenticated/projects/$projectId')({
  ssr: false,
  loader: async ({ params }) => {
    const [topology, hosts] = await Promise.all([getProjectTopology(params.projectId), listProjectHosts(params.projectId)])
    return { ...topology, hosts }
  },
  component: ProjectCanvasPage,
})

interface CommandState {
  hostID: string
  command: string
}

interface HostConnectionProps {
  hostID: string
  commandState: CommandState | null
  onCommand: (command: string) => void
}

function commandKey(hostID: string) {
  return `orca.host-command.${hostID}`
}

function maskCommand(command: string) {
  return command.replace(/(export ORCA_TOKEN=')([^']+)(')/, (_, before: string, token: string, after: string) => `${before}${token.slice(0, 8)}••••••••${token.slice(-4)}${after}`)
}

function ProjectCanvasPage() {
  const initialTopology = Route.useLoaderData()
  const { project } = initialTopology
  const [clusters, setClusters] = useState(initialTopology.clusters)
  const { projectId } = Route.useParams()
  const [hostState, setHostState] = useState({ projectID: projectId, hosts: initialTopology.hosts })
  const snapshot = useTopologyStore((state) => state.snapshot)
  useProjectEvents(projectId)
  useEffect(() => setClusters(initialTopology.clusters), [initialTopology.clusters, projectId])
  const projectSnapshot = snapshot?.project_id === projectId ? snapshot : null
  const [now, setNow] = useState(() => Date.now())
  const fresh = projectSnapshot !== null && areReportsFresh(projectSnapshot.clusters, now)
  const hosts = hostState.projectID === projectId ? hostState.hosts : initialTopology.hosts
  const awaitingHost = hosts.find((host) => host.status === 'never_connected')
  const [commandState, setCommandState] = useState<CommandState | null>(null)
  const restart = useRestartProject(projectId)
  useEffect(() => setHostState({ projectID: projectId, hosts: initialTopology.hosts }), [initialTopology.hosts, projectId])
  useEffect(() => {
    if (projectSnapshot?.desired_clusters) setClusters(projectSnapshot.desired_clusters)
  }, [projectSnapshot?.desired_clusters])
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 15_000)
    return () => window.clearInterval(timer)
  }, [])
  useEffect(() => {
    let active = true
    const refresh = () => void listProjectHosts(projectId).then((nextHosts) => {
      if (active) setHostState({ projectID: projectId, hosts: nextHosts })
    }).catch(() => undefined)
    const timer = window.setInterval(refresh, 10_000)
    return () => { active = false; window.clearInterval(timer) }
  }, [projectId])
  useEffect(() => {
    if (!awaitingHost) {
      setCommandState((current) => {
        if (current) {
          window.sessionStorage.removeItem(commandKey(current.hostID))
        }
        return null
      })
      return
    }
    const command = window.sessionStorage.getItem(commandKey(awaitingHost.id))
    if (command) {
      setCommandState({ hostID: awaitingHost.id, command })
      return
    }
    setCommandState(null)
  }, [awaitingHost?.id])

  function storeCommand(command: string) {
    if (!awaitingHost) return
    window.sessionStorage.setItem(commandKey(awaitingHost.id), command)
    setCommandState({ hostID: awaitingHost.id, command })
  }

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
          <button type="button" disabled={restart.restarting || clusters.length === 0} onClick={restart.openDialog} className="inline-flex items-center gap-2 rounded-full border border-[var(--warning)]/40 bg-[var(--warning)]/10 px-3.5 py-2 text-[11px] font-medium text-[var(--warning)] hover:bg-[var(--warning)]/15 disabled:cursor-not-allowed disabled:opacity-40"><RefreshCw className={`h-3.5 w-3.5 ${restart.restarting ? 'animate-spin' : ''}`} />{restart.restarting ? 'Requesting...' : 'Restart'}</button>
          <Link to="/projects/$projectId/settings" params={{ projectId }} className="inline-flex items-center gap-2 rounded-full border border-[var(--border)] bg-[var(--panel)] px-3.5 py-2 text-[11px] font-medium text-[var(--text-2)] hover:border-[var(--text-3)] hover:text-[var(--text)]"><Settings2 className="h-3.5 w-3.5" />Settings</Link>
          <div className="flex items-center gap-2.5 rounded-full border border-[var(--border)] bg-[var(--panel)] px-3.5 py-2 text-[11px] font-medium text-[var(--text-2)] shadow-[inset_0_1px_rgba(255,255,255,0.03)]">
            <span className={`h-1.5 w-1.5 rounded-full ${fresh ? 'bg-[var(--healthy)]' : 'bg-[var(--text-3)]'}`} />
            {fresh ? 'Live telemetry' : 'Telemetry stale or unavailable'}
          </div>
        </div>
      </header>
      {restart.message && <p role={restart.failed ? 'alert' : 'status'} className={`mb-3 rounded-[var(--radius-md)] border px-3 py-2 text-xs ${restart.failed ? 'border-[var(--critical)]/30 bg-[var(--critical)]/5 text-[var(--critical)]' : 'border-[var(--warning)]/30 bg-[var(--warning)]/5 text-[var(--warning)]'}`}>{restart.message}</p>}
      <RestartProjectDialog open={restart.dialogOpen} clusterCount={clusters.length} restarting={restart.restarting} onCancel={restart.closeDialog} onConfirm={restart.requestRestart} />
      {awaitingHost && <HostConnection hostID={awaitingHost.id} commandState={commandState?.hostID === awaitingHost.id ? commandState : null} onCommand={storeCommand} />}
      {clusters.length === 0 ? (
        <ConnectHostEmptyState className="flex-1" title="Connect a host to get started" description="Register an Orca agent on the infrastructure that will run PostgreSQL. Once the host connects, you can configure this project's topology." />
      ) : (
        <CanvasView key={projectId} clusters={clusters} snapshot={projectSnapshot} onClusterUpdated={(updated) => setClusters((current) => current.map((cluster) => cluster.id === updated.id ? updated : cluster))} />
      )}
    </main>
  )
}

function HostConnection({ hostID, commandState, onCommand }: HostConnectionProps) {
  const [copied, setCopied] = useState(false)
  const [generating, setGenerating] = useState(false)
  const [error, setError] = useState('')
  const [revealed, setRevealed] = useState(false)
  useEffect(() => {
    setRevealed(false)
  }, [commandState?.command])
  useEffect(() => {
    if (!revealed) return
    const timer = window.setTimeout(() => setRevealed(false), 10_000)
    return () => window.clearTimeout(timer)
  }, [revealed])
  async function copyCommand() {
    if (!commandState) return
    await navigator.clipboard.writeText(commandState.command)
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
  const displayedCommand = commandState ? revealed ? commandState.command : maskCommand(commandState.command) : ''
  return <section className="mb-3 rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--card)] p-4"><div className="flex flex-col gap-4 lg:flex-row lg:items-center"><div className="flex min-w-0 flex-1 gap-3"><span className="grid h-9 w-9 shrink-0 place-items-center rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--panel)] text-[var(--accent)]"><Terminal className="h-4 w-4" /></span><div><h2 className="text-sm font-semibold">Connect the registered host</h2><p className="mt-1 text-xs leading-5 text-[var(--text-2)]">Run this one-time command on the host that will provide storage and compute. Desired topology and extensions are waiting for its agent.</p></div></div>{commandState ? <button type="button" onClick={copyCommand} className="inline-flex shrink-0 items-center justify-center gap-2 rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--panel)] px-3.5 py-2 text-xs font-medium text-[var(--text-2)] hover:border-[var(--accent)] hover:text-[var(--text)]"><Copy className="h-3.5 w-3.5" />{copied ? 'Copied' : 'Copy command'}</button> : <button type="button" disabled={generating} onClick={generateCommand} className="inline-flex shrink-0 items-center justify-center gap-2 rounded-[var(--radius-sm)] bg-[var(--accent)] px-3.5 py-2 text-xs font-semibold text-[var(--accent-contrast)] hover:bg-[var(--accent-hover)] disabled:opacity-50"><RefreshCw className="h-3.5 w-3.5" />{generating ? 'Generating...' : 'Generate command'}</button>}</div>{error && <p role="alert" className="mt-3 text-xs text-[var(--critical)]">{error}</p>}{commandState && <><p className="mt-3 text-[11px] text-[var(--text-3)]">Requires the Orca repo checked out locally — a prebuilt Docker image is coming soon</p><div className="relative mt-2"><pre className="overflow-x-auto rounded-[var(--radius-sm)] bg-[var(--panel)] p-3 pr-24 font-mono text-[11px] leading-5 text-[var(--text-2)]"><code>{displayedCommand}</code></pre><button type="button" aria-label={revealed ? 'Hide token' : 'Reveal token'} onClick={() => setRevealed((current) => !current)} className="absolute right-2 top-2 inline-flex items-center gap-1.5 rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--card)] px-2 py-1 text-[10px] font-medium text-[var(--text-2)] hover:border-[var(--accent)] hover:text-[var(--text)]">{revealed ? <EyeOff className="h-3 w-3" /> : <Eye className="h-3 w-3" />}{revealed ? 'Hide' : 'Reveal'}</button></div><button type="button" disabled={generating} onClick={generateCommand} className="mt-2 text-[11px] text-[var(--text-3)] hover:text-[var(--text-2)]">{generating ? 'Regenerating...' : 'Regenerate and invalidate this token'}</button></>}</section>
}
