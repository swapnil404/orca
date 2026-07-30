import { Link, createFileRoute, useNavigate } from '@tanstack/react-router'
import { Check, ChevronLeft, ChevronRight, Database, Info, Minus, Network, Plus, Server, X } from 'lucide-react'
import { useState } from 'react'
import { ApiError, createCluster, createProject, deleteProject, deleteUnusedHost, enablePgBouncer, listOrganizations, registerHost, rotateHostToken } from '../../../api'
import type { PgBouncerPoolMode } from '../../../types/resources'

const projectNamePattern = /^[a-z0-9]+(?:-[a-z0-9]+)*$/
const maxReplicaCount = 10
const steps = ['Instance Config', 'Topology', 'Review'] as const
const creationPhaseLabels = {
  project: 'Creating project',
  host: 'Registering host slot',
  cluster: 'Provisioning cluster',
} as const
const postgresVersions = ['17', '16', '15'] as const
const extensions = [
  { id: 'pgvector', name: 'pgvector', description: 'Vector similarity search' },
  { id: 'timescaledb', name: 'TimescaleDB', description: 'Time-series data and analytics' },
  { id: 'postgis', name: 'PostGIS', description: 'Geospatial data and queries' },
  { id: 'pg_partman', name: 'pg_partman', description: 'Automated table partitioning' },
  { id: 'powa', name: 'PoWA', description: 'PostgreSQL workload analysis' },
] as const

type ExtensionID = (typeof extensions)[number]['id']
type Topology = 'replicas' | 'single'
type CreationPhase = keyof typeof creationPhaseLabels

export const Route = createFileRoute('/_authenticated/projects/new')({
  ssr: false,
  loader: listOrganizations,
  component: NewProjectPage,
})

function NewProjectPage() {
  const organizations = Route.useLoaderData()
  const navigate = useNavigate()
  const [step, setStep] = useState(0)
  const [organizationID, setOrganizationID] = useState(organizations[0]?.id ?? '')
  const [name, setName] = useState('')
  const [postgresVersion, setPostgresVersion] = useState<(typeof postgresVersions)[number]>('17')
  const [selectedExtensions, setSelectedExtensions] = useState<ExtensionID[]>([])
  const [topology, setTopology] = useState<Topology>('replicas')
  const [replicaCount, setReplicaCount] = useState(1)
  const [poolingEnabled, setPoolingEnabled] = useState(false)
  const [poolMode, setPoolMode] = useState<PgBouncerPoolMode>('transaction')
  const [maxConnections, setMaxConnections] = useState(100)
  const [submitting, setSubmitting] = useState(false)
  const [creationPhase, setCreationPhase] = useState<CreationPhase | null>(null)
  const [error, setError] = useState('')
  const nameIsValid = projectNamePattern.test(name)
  const instanceConfigIsValid = nameIsValid && organizationID.length > 0
  const effectiveReplicaCount = topology === 'replicas' ? replicaCount : 0
  const poolingIsValid = !poolingEnabled || (Number.isInteger(maxConnections) && maxConnections > 0)
  const selectedOrganization = organizations.find((organization) => organization.id === organizationID)

  function toggleExtension(extension: ExtensionID) {
    setSelectedExtensions((current) => current.includes(extension) ? current.filter((item) => item !== extension) : [...current, extension])
  }

  function goNext() {
    if (step === 0 && !instanceConfigIsValid) return
    setStep((current) => Math.min(current + 1, steps.length - 1))
  }

  async function handleCreate() {
    if (!instanceConfigIsValid || submitting) return
    setSubmitting(true)
    setError('')
    let failedPhase: CreationPhase = 'project'
    let projectID = ''
    let hostID = ''
    try {
      setCreationPhase('project')
      const project = await createProject(name, organizationID)
      projectID = project.id
      failedPhase = 'host'
      setCreationPhase('host')
      hostID = crypto.randomUUID()
      let host
      try {
        host = await registerHost(hostID)
      } catch (registrationError) {
        try {
          host = await rotateHostToken(hostID)
        } catch (recoveryError) {
          if (recoveryError instanceof ApiError && recoveryError.status === 404) hostID = ''
          throw registrationError
        }
      }
      window.sessionStorage.setItem(`orca.host-command.${host.host_id}`, host.docker_run_command)
      failedPhase = 'cluster'
      setCreationPhase('cluster')
      const cluster = await createCluster(project.id, {
        host_id: host.host_id,
        name: 'main',
        postgres_version: postgresVersion,
        parameters: {},
        replica_count: effectiveReplicaCount,
        enabled_extensions: selectedExtensions,
        pgbouncer_enabled: false,
      })
      if (poolingEnabled) await enablePgBouncer(cluster, { pool_mode: poolMode, max_connections: maxConnections })
    } catch (cause) {
      const rollback = []
      if (projectID) {
        try {
          await deleteProject(projectID)
          rollback.push('project removed')
        } catch {
          rollback.push('project cleanup failed')
        }
      }
      if (hostID) {
        window.sessionStorage.removeItem(`orca.host-command.${hostID}`)
        try {
          await deleteUnusedHost(hostID)
          rollback.push('host registration revoked')
        } catch (cleanupError) {
          if (cleanupError instanceof ApiError && cleanupError.status === 404) rollback.push('host registration absent')
          else rollback.push('host cleanup failed')
        }
      }
      const ambiguousCreate = !projectID && !(cause instanceof ApiError)
      const detail = cause instanceof ApiError ? cause.message : 'The server could not complete the request.'
      const cleanupComplete = rollback.every((result) => !result.includes('failed'))
      const rollbackStatus = rollback.length === 0 ? 'No rollback was possible.' : cleanupComplete ? `Rollback completed (${rollback.join(', ')}).` : `Rollback incomplete (${rollback.join(', ')}).`
      const recovery = ambiguousCreate || !cleanupComplete ? ' Check existing resources before trying again; the request outcome could not be confirmed.' : ''
      setError(`${creationPhaseLabels[failedPhase]} failed: ${detail} ${rollbackStatus}${recovery}`)
      setSubmitting(ambiguousCreate || !cleanupComplete)
      if (!ambiguousCreate && cleanupComplete) setCreationPhase(null)
      return
    }
    await navigate({ to: '/projects/$projectId', params: { projectId: projectID } }).catch(() => window.location.assign(`/projects/${encodeURIComponent(projectID)}`))
  }

  return (
    <main className="min-h-[calc(100vh-56px)] px-4 py-8 text-[var(--text)] sm:px-6 lg:px-8 lg:py-10">
      <div className="mx-auto max-w-6xl">
        <header className="flex items-start justify-between gap-4">
          <div><p className="font-mono text-[10px] font-medium uppercase tracking-[0.16em] text-[var(--text-3)]">New project</p><h1 className="mt-2 text-2xl font-semibold tracking-tight">Create a project</h1></div>
          <Link to="/" aria-label="Cancel project creation" className="grid h-9 w-9 shrink-0 place-items-center rounded-full border border-[var(--border)] bg-[var(--panel)] text-[var(--text-2)] hover:border-[var(--text-3)] hover:text-[var(--text)]"><X aria-hidden="true" className="h-4 w-4" /></Link>
        </header>

        <StepIndicator current={step} />

        <div className="mt-9 grid items-start gap-8 lg:grid-cols-[minmax(0,1fr)_360px]">
          <section className="min-w-0">
            <div className="mb-6"><p className="font-mono text-[10px] uppercase tracking-[0.14em] text-[var(--text-3)]">Step {step + 1} of {steps.length}</p><h2 className="mt-1.5 text-xl font-semibold">{steps[step]}</h2></div>

            {step === 0 && <InstanceConfig organizationID={organizationID} organizations={organizations} name={name} nameIsValid={nameIsValid} postgresVersion={postgresVersion} selectedExtensions={selectedExtensions} onOrganizationChange={setOrganizationID} onNameChange={setName} onVersionChange={setPostgresVersion} onToggleExtension={toggleExtension} />}
            {step === 1 && <><TopologyStep topology={topology} replicaCount={replicaCount} onTopologyChange={setTopology} onReplicaCountChange={setReplicaCount} /><PoolingSettings enabled={poolingEnabled} poolMode={poolMode} maxConnections={maxConnections} onEnabledChange={setPoolingEnabled} onPoolModeChange={setPoolMode} onMaxConnectionsChange={setMaxConnections} /></>}
            {step === 2 && <Review organization={selectedOrganization?.name ?? 'Not selected'} name={name} postgresVersion={postgresVersion} replicaCount={effectiveReplicaCount} selectedExtensions={selectedExtensions} poolingEnabled={poolingEnabled} poolMode={poolMode} maxConnections={maxConnections} />}

            {step === steps.length - 1 && error && <div role="alert" className="mt-6 rounded-[var(--radius-sm)] border border-[var(--critical)]/40 bg-[var(--critical)]/5 px-4 py-3 text-sm text-[var(--critical)]">{error}</div>}

            <div className="mt-8 flex items-center justify-between border-t border-[var(--border)] pt-5">
              <button type="button" disabled={step === 0 || submitting} onClick={() => setStep((current) => Math.max(0, current - 1))} className="inline-flex items-center gap-2 rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--panel)] px-4 py-2.5 text-sm font-medium text-[var(--text-2)] hover:border-[var(--text-3)] hover:text-[var(--text)] disabled:cursor-not-allowed disabled:opacity-35"><ChevronLeft className="h-4 w-4" />Back</button>
              {step < steps.length - 1 ? <button type="button" disabled={(step === 0 && !instanceConfigIsValid) || (step === 1 && !poolingIsValid) || submitting} onClick={goNext} className="inline-flex items-center gap-2 rounded-[var(--radius-sm)] bg-[var(--accent)] px-5 py-2.5 text-sm font-semibold text-[var(--accent-contrast)] hover:bg-[var(--accent-hover)] active:translate-y-px disabled:cursor-not-allowed disabled:opacity-45">Next: {steps[step + 1]}<ChevronRight className="h-4 w-4" /></button> : <button type="button" disabled={!instanceConfigIsValid || !poolingIsValid || submitting} onClick={handleCreate} className="inline-flex min-w-48 items-center justify-center rounded-[var(--radius-sm)] bg-[var(--accent)] px-5 py-2.5 text-sm font-semibold text-[var(--bg)] hover:bg-[var(--accent-hover)] active:translate-y-px disabled:cursor-not-allowed disabled:opacity-45">{creationPhase ? `${creationPhaseLabels[creationPhase]}...` : 'Create project'}</button>}
            </div>
          </section>

          <ProjectSummary organization={selectedOrganization?.name} name={name} postgresVersion={postgresVersion} replicaCount={effectiveReplicaCount} selectedExtensions={selectedExtensions} poolingEnabled={poolingEnabled} poolMode={poolMode} maxConnections={maxConnections} />
        </div>
      </div>
    </main>
  )
}

function StepIndicator({ current }: { current: number }) {
  return <ol aria-label="Project creation progress" className="mt-8 flex max-w-2xl items-start">{steps.map((label, index) => { const complete = index < current; const active = index === current; return <li key={label} aria-current={active ? 'step' : undefined} className={`relative flex flex-1 flex-col items-center text-center last:flex-none ${index < steps.length - 1 ? 'after:absolute after:left-[calc(50%+20px)] after:top-4 after:h-px after:w-[calc(100%-40px)] after:bg-[var(--border)]' : ''}`}><span className={`relative z-10 grid h-8 w-8 place-items-center rounded-full border font-mono text-xs ${complete ? 'border-[var(--healthy)] bg-[var(--panel)] text-[var(--healthy)]' : active ? 'border-[var(--accent)] bg-[var(--accent-soft)] text-[var(--accent)]' : 'border-[var(--border)] bg-[var(--panel)] text-[var(--text-3)]'}`}>{complete ? <Check className="h-4 w-4" /> : index + 1}</span><span className={`mt-2 hidden text-xs sm:block ${complete ? 'text-[var(--healthy)]' : active ? 'text-[var(--text)]' : 'text-[var(--text-3)]'}`}>{label}</span></li> })}</ol>
}

interface InstanceConfigProps {
  organizationID: string
  organizations: Awaited<ReturnType<typeof listOrganizations>>
  name: string
  nameIsValid: boolean
  postgresVersion: (typeof postgresVersions)[number]
  selectedExtensions: ExtensionID[]
  onOrganizationChange: (value: string) => void
  onNameChange: (value: string) => void
  onVersionChange: (value: (typeof postgresVersions)[number]) => void
  onToggleExtension: (value: ExtensionID) => void
}

function InstanceConfig({ organizationID, organizations, name, nameIsValid, postgresVersion, selectedExtensions, onOrganizationChange, onNameChange, onVersionChange, onToggleExtension }: InstanceConfigProps) {
  const showNameError = name.length > 0 && !nameIsValid
  return <div className="space-y-7"><label className="block text-sm font-medium">Organization<select required value={organizationID} onChange={(event) => onOrganizationChange(event.target.value)} className="mt-2 w-full rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--panel)] px-3 py-2.5 text-sm outline-none focus:border-[var(--accent)] focus:ring-2 focus:ring-[var(--accent-soft)]">{organizations.length === 0 && <option value="">No organizations available</option>}{organizations.map((organization) => <option key={organization.id} value={organization.id}>{organization.name}</option>)}</select></label><label className="block text-sm font-medium">Project name<input autoFocus required value={name} onChange={(event) => onNameChange(event.target.value)} aria-invalid={showNameError} aria-describedby="project-name-note" maxLength={100} placeholder="production-database" className={`mt-2 w-full rounded-[var(--radius-sm)] border bg-[var(--panel)] px-3 py-2.5 font-mono text-sm outline-none focus:ring-2 focus:ring-[var(--accent-soft)] ${showNameError ? 'border-[var(--critical)]' : 'border-[var(--border)] focus:border-[var(--accent)]'}`} /><span id="project-name-note" className={`mt-2 block text-xs ${showNameError ? 'text-[var(--critical)]' : 'text-[var(--text-3)]'}`}>Lowercase alphanumeric characters and dashes only.</span></label><label className="block text-sm font-medium">PostgreSQL version<select value={postgresVersion} onChange={(event) => onVersionChange(event.target.value as (typeof postgresVersions)[number])} className="mt-2 w-full rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--panel)] px-3 py-2.5 font-mono text-sm outline-none focus:border-[var(--accent)] focus:ring-2 focus:ring-[var(--accent-soft)]">{postgresVersions.map((version) => <option key={version} value={version}>PostgreSQL {version}</option>)}</select></label><fieldset><legend className="text-sm font-medium">Extensions</legend><p className="mt-1 text-xs text-[var(--text-3)]">Desired extensions install when the agent first reconciles this cluster.</p><div className="mt-3 grid gap-2 sm:grid-cols-2">{extensions.map((extension) => <label key={extension.id} className={`flex cursor-pointer gap-3 rounded-[var(--radius-md)] border p-3.5 ${selectedExtensions.includes(extension.id) ? 'border-[var(--accent)] bg-[var(--accent-soft)]' : 'border-[var(--border)] bg-[var(--card)] hover:border-[var(--text-3)]'}`}><input type="checkbox" checked={selectedExtensions.includes(extension.id)} onChange={() => onToggleExtension(extension.id)} className="mt-0.5 h-4 w-4 accent-[var(--accent)]" /><span><span className="block text-sm font-medium">{extension.name}</span><span className="mt-1 block text-xs text-[var(--text-3)]">{extension.description}</span></span></label>)}</div></fieldset></div>
}

interface TopologyStepProps { topology: Topology; replicaCount: number; onTopologyChange: (value: Topology) => void; onReplicaCountChange: (value: number) => void }

function TopologyStep({ topology, replicaCount, onTopologyChange, onReplicaCountChange }: TopologyStepProps) {
  return <div><p className="text-sm text-[var(--text-2)]">Choose the desired PostgreSQL node layout. It will reconcile when the registered host connects.</p><div className="mt-5 grid gap-3 sm:grid-cols-2"><button type="button" aria-pressed={topology === 'replicas'} onClick={() => onTopologyChange('replicas')} className={`rounded-[var(--radius-lg)] border p-5 text-left ${topology === 'replicas' ? 'border-[var(--accent)] bg-[var(--accent-soft)]' : 'border-[var(--border)] bg-[var(--card)] hover:border-[var(--text-3)]'}`}><span className="grid h-9 w-9 place-items-center rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--panel)] text-[var(--streaming)]"><Network className="h-4 w-4" /></span><span className="mt-4 block text-sm font-semibold">Primary + replicas</span><span className="mt-1.5 block text-xs leading-5 text-[var(--text-2)]">One writable primary with streaming read replicas.</span></button><button type="button" aria-pressed={topology === 'single'} onClick={() => onTopologyChange('single')} className={`rounded-[var(--radius-lg)] border p-5 text-left ${topology === 'single' ? 'border-[var(--accent)] bg-[var(--accent-soft)]' : 'border-[var(--border)] bg-[var(--card)] hover:border-[var(--text-3)]'}`}><span className="grid h-9 w-9 place-items-center rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--panel)] text-[var(--accent)]"><Database className="h-4 w-4" /></span><span className="mt-4 block text-sm font-semibold">Single node</span><span className="mt-1.5 block text-xs leading-5 text-[var(--text-2)]">One writable primary with no replicas.</span></button></div>{topology === 'replicas' && <div className="mt-5 flex items-center justify-between rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--panel)] p-4"><div><p className="text-sm font-medium">Replica count</p><p className="mt-1 text-xs text-[var(--text-3)]">Streaming replicas created alongside the primary.</p></div><div className="flex items-center rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--card)]"><button type="button" aria-label="Remove replica" disabled={replicaCount <= 1} onClick={() => onReplicaCountChange(Math.max(1, replicaCount - 1))} className="grid h-9 w-9 place-items-center text-[var(--text-2)] hover:text-[var(--text)] disabled:opacity-30"><Minus className="h-3.5 w-3.5" /></button><output aria-live="polite" className="min-w-12 border-x border-[var(--border)] px-3 text-center font-mono text-sm">{replicaCount}</output><button type="button" aria-label="Add replica" disabled={replicaCount >= maxReplicaCount} onClick={() => onReplicaCountChange(Math.min(maxReplicaCount, replicaCount + 1))} className="grid h-9 w-9 place-items-center text-[var(--text-2)] hover:text-[var(--text)] disabled:opacity-30"><Plus className="h-3.5 w-3.5" /></button></div></div>}</div>
}

interface PoolingSettingsProps {
  enabled: boolean
  poolMode: PgBouncerPoolMode
  maxConnections: number
  onEnabledChange: (value: boolean) => void
  onPoolModeChange: (value: PgBouncerPoolMode) => void
  onMaxConnectionsChange: (value: number) => void
}

function PoolingSettings({ enabled, poolMode, maxConnections, onEnabledChange, onPoolModeChange, onMaxConnectionsChange }: PoolingSettingsProps) {
  return <div className="mt-5 rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--panel)] p-4"><label className="flex items-start gap-3"><input type="checkbox" checked={enabled} onChange={(event) => onEnabledChange(event.target.checked)} className="mt-0.5 h-4 w-4 accent-[var(--accent)]" /><span><span className="block text-sm font-medium">Enable connection pooling</span><span className="mt-1 block text-xs leading-5 text-[var(--text-3)]">Add PgBouncer to desired state. It remains pending until the host connects.</span></span></label>{enabled && <div className="mt-4 grid gap-4 border-t border-[var(--border-soft)] pt-4 sm:grid-cols-2"><label className="text-xs font-medium text-[var(--text-2)]">Pool mode<select value={poolMode} onChange={(event) => onPoolModeChange(event.target.value as PgBouncerPoolMode)} className="mt-2 w-full rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--card)] px-3 py-2.5 font-mono text-sm text-[var(--text)] outline-none focus:border-[var(--accent)]"><option value="session">session</option><option value="transaction">transaction</option><option value="statement">statement</option></select></label><label className="text-xs font-medium text-[var(--text-2)]">Max connections<input type="number" min="1" step="1" required value={maxConnections} onChange={(event) => onMaxConnectionsChange(event.target.valueAsNumber)} className="mt-2 w-full rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--card)] px-3 py-2.5 font-mono text-sm text-[var(--text)] outline-none focus:border-[var(--accent)]" /></label></div>}</div>
}

function Review({ organization, name, postgresVersion, replicaCount, selectedExtensions, poolingEnabled, poolMode, maxConnections }: { organization: string; name: string; postgresVersion: string; replicaCount: number; selectedExtensions: ExtensionID[]; poolingEnabled: boolean; poolMode: PgBouncerPoolMode; maxConnections: number }) {
  const rows = [['Organization', organization], ['Project name', name], ['PostgreSQL', `Version ${postgresVersion}`], ['Topology', replicaCount === 0 ? '1 primary' : `1 primary + ${replicaCount} ${replicaCount === 1 ? 'replica' : 'replicas'}`], ['Extensions', selectedExtensions.length > 0 ? selectedExtensions.join(', ') : 'None'], ['Connection pooling', poolingEnabled ? `${poolMode}, ${maxConnections} max connections (pending host connection)` : 'Disabled']]
  return <div className="overflow-hidden rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--card)]"><div className="divide-y divide-[var(--border-soft)]">{rows.map(([label, value]) => <div key={label} className="grid gap-1 px-5 py-4 sm:grid-cols-[150px_1fr]"><span className="text-xs text-[var(--text-3)]">{label}</span><span className="text-sm text-[var(--text)]">{value}</span></div>)}</div></div>
}

function ProjectSummary({ organization, name, postgresVersion, replicaCount, selectedExtensions, poolingEnabled, poolMode, maxConnections }: { organization?: string; name: string; postgresVersion: string; replicaCount: number; selectedExtensions: ExtensionID[]; poolingEnabled: boolean; poolMode: PgBouncerPoolMode; maxConnections: number }) {
  return <aside className="rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--card)] p-5 lg:sticky lg:top-8"><h2 className="text-sm font-semibold">Your project includes</h2><div className="mt-4 divide-y divide-[var(--border-soft)] border-y border-[var(--border-soft)]"><SummaryRow icon={Server} label={name || 'Unnamed project'} detail={organization ?? 'No organization selected'} /><SummaryRow icon={Database} label={replicaCount === 0 ? '1 primary' : `1 primary + ${replicaCount} ${replicaCount === 1 ? 'replica' : 'replicas'}`} detail={`PostgreSQL ${postgresVersion}`} /><SummaryRow icon={Plus} label={`${selectedExtensions.length} ${selectedExtensions.length === 1 ? 'extension' : 'extensions'}`} detail={selectedExtensions.length > 0 ? selectedExtensions.join(', ') : 'None selected'} /><SummaryRow icon={Network} label={poolingEnabled ? 'Connection pooling enabled' : 'Connection pooling disabled'} detail={poolingEnabled ? `${poolMode}, ${maxConnections} max connections` : 'No PgBouncer desired state'} /></div><div className="mt-5 flex gap-3 rounded-[var(--radius-md)] bg-[var(--panel)] p-4 text-xs leading-5 text-[var(--text-2)]"><Info aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0 text-[var(--accent)]" /><p>Storage and compute are provided by the host you'll connect after creating this project.</p></div></aside>
}

function SummaryRow({ icon: Icon, label, detail }: { icon: typeof Server; label: string; detail: string }) {
  return <div className="flex items-center gap-3 py-3.5"><Icon aria-hidden="true" className="h-4 w-4 shrink-0 text-[var(--text-3)]" /><div className="min-w-0"><p className="truncate text-sm font-medium">{label}</p><p className="mt-0.5 truncate text-xs text-[var(--text-3)]">{detail}</p></div></div>
}
