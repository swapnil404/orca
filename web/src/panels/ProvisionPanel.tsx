import { useState, type FormEvent } from 'react'
import type { Cluster, ProjectStateSnapshot } from '../types/resources'
import type { PaletteNodeType, ProvisionRequest } from '../components/topology/types'
import { Drawer } from '../components/shell/Drawer'
import { PanelLayout } from './PanelLayout'

interface ProvisionPanelProps {
  type: PaletteNodeType | null
  clusters: Cluster[]
  snapshot: ProjectStateSnapshot | null
  busy: boolean
  error?: string
  onCancel: () => void
  onClose: () => void
  onConfirm: (request: ProvisionRequest) => void
}

const fieldClass = 'mt-2 w-full rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--panel)] px-3 py-2.5 font-mono text-sm text-[var(--text)] outline-none hover:border-[var(--text-3)] focus:border-[var(--accent)] focus:ring-2 focus:ring-[var(--accent-soft)] disabled:cursor-not-allowed disabled:opacity-50'
const buttonClass = 'inline-flex items-center justify-center rounded-[var(--radius-md)] px-4 py-2.5 text-sm font-medium focus:outline-none focus:ring-2 focus:ring-[var(--accent)] disabled:cursor-not-allowed disabled:opacity-50'

const labels: Record<PaletteNodeType, { title: string; eyebrow: string }> = {
  replica: { title: 'Add replica', eyebrow: 'Database desired state' },
  pgbouncer: { title: 'Provision PgBouncer', eyebrow: 'Connection pooler desired state' },
  pgbackrest: { title: 'Configure pgBackRest', eyebrow: 'Backup desired state' },
  extension: { title: 'Install extension', eyebrow: 'Extension desired state' },
}

export function ProvisionPanel({ type, clusters, snapshot, busy, error, onCancel, onClose, onConfirm }: ProvisionPanelProps) {
  const [clusterID, setClusterID] = useState('')
  const eligibleClusters = type === 'pgbouncer' ? clusters.filter((cluster) => !cluster.pgbouncer_enabled)
    : type === 'pgbackrest' ? clusters.filter((cluster) => !cluster.pg_back_rest)
      : type === 'extension' ? clusters.filter((cluster) => collectExtensionNames(snapshot, cluster).length > 0)
      : clusters
  const selectedClusterID = eligibleClusters.some((cluster) => cluster.id === clusterID) ? clusterID : eligibleClusters[0]?.id ?? ''
  const selectedCluster = clusters.find((cluster) => cluster.id === selectedClusterID)
  const extensionNames = selectedCluster ? collectExtensionNames(snapshot, selectedCluster) : []

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!type || !selectedClusterID) return
    const form = new FormData(event.currentTarget)
    if (type === 'replica') onConfirm({ type, clusterID: selectedClusterID })
    if (type === 'pgbouncer') onConfirm({ type, clusterID: selectedClusterID, poolMode: String(form.get('pool_mode')) as 'session' | 'transaction' | 'statement', maxConnections: Number(form.get('max_connections')), publishAddress: String(form.get('publish_address')), publishPort: Number(form.get('publish_port')) })
    if (type === 'pgbackrest') onConfirm({ type, clusterID: selectedClusterID, repoPath: String(form.get('repo_path') ?? '').trim(), retentionFull: Number(form.get('retention_full')), retentionDiff: Number(form.get('retention_diff')), fullIntervalSeconds: Number(form.get('full_interval_seconds')), diffIntervalSeconds: Number(form.get('diff_interval_seconds')), incrIntervalSeconds: Number(form.get('incr_interval_seconds')) })
    if (type === 'extension') onConfirm({ type, clusterID: selectedClusterID, extension: String(form.get('extension') ?? '') })
  }

  const meta = type ? labels[type] : null
  const noCapabilities = type === 'extension' && extensionNames.length === 0
  return <Drawer open={type !== null} onClose={busy ? () => undefined : onClose}>{meta && type ? <PanelLayout title={meta.title} eyebrow={meta.eyebrow} onClose={busy ? () => undefined : onClose}>
    <form onSubmit={submit} className="space-y-5">
      <p className="text-sm leading-6 text-[var(--text-2)]">This writes the selected cluster's real desired-state contract. Health changes only after fresh agent telemetry confirms the resource.</p>
      <label className="block text-xs font-medium text-[var(--text-2)]">Target cluster<select required value={selectedClusterID} onChange={(event) => setClusterID(event.target.value)} disabled={busy} className={fieldClass}>{eligibleClusters.map((cluster) => <option key={cluster.id} value={cluster.id}>{cluster.name}</option>)}</select></label>
      {type === 'replica' ? <p className="rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--panel)] p-3 text-xs leading-5 text-[var(--text-3)]">The current replica contract has no size or region fields. The server assigns the replica ID and provisions streaming replication on the primary's host.</p> : null}
      {type === 'pgbouncer' ? <div className="grid gap-4 sm:grid-cols-2"><label className="text-xs font-medium text-[var(--text-2)]">Pool mode<select name="pool_mode" defaultValue="transaction" disabled={busy} className={fieldClass}><option value="session">Session</option><option value="transaction">Transaction</option><option value="statement">Statement</option></select></label><NumberField name="max_connections" label="Max connections" value={100} min={1} disabled={busy} /><label className="text-xs font-medium text-[var(--text-2)]">Publish address<input required name="publish_address" defaultValue="127.0.0.1" disabled={busy} className={fieldClass} /></label><NumberField name="publish_port" label="Publish port" value={6432} min={1} disabled={busy} /></div> : null}
      {type === 'pgbackrest' ? <><label className="block text-xs font-medium text-[var(--text-2)]">Repository path<input required name="repo_path" placeholder="Path on the agent host" disabled={busy} className={fieldClass} /></label><p className="-mt-3 text-[11px] leading-5 text-[var(--text-3)]">The current backend supports a local repository path only; no standalone S3 target exists.</p><div className="grid gap-4 sm:grid-cols-2"><NumberField name="retention_full" label="Full backups retained" value={2} min={1} disabled={busy} /><NumberField name="retention_diff" label="Diff backups retained" value={4} min={1} disabled={busy} /></div><div className="grid gap-4 sm:grid-cols-3"><NumberField name="full_interval_seconds" label="Full interval" unit="sec" value={86400} min={0} disabled={busy} /><NumberField name="diff_interval_seconds" label="Diff interval" unit="sec" value={21600} min={0} disabled={busy} /><NumberField name="incr_interval_seconds" label="Incr interval" unit="sec" value={3600} min={0} disabled={busy} /></div></> : null}
      {type === 'extension' ? <>{noCapabilities ? <p className="rounded-[var(--radius-md)] border border-[var(--warning)]/25 bg-[var(--warning)]/5 p-3 text-xs leading-5 text-[var(--text-2)]">No extension capability has been reported by the connected agents, so no extension name is guessed.</p> : <label className="block text-xs font-medium text-[var(--text-2)]">Reported extension<select required name="extension" disabled={busy} className={fieldClass}>{extensionNames.map((name) => <option key={name}>{name}</option>)}</select></label>}</> : null}
      {eligibleClusters.length === 0 ? <p role="alert" className="text-xs text-[var(--warning)]">No eligible cluster exists for this resource.</p> : null}
      {error ? <p role="alert" className="rounded-[var(--radius-md)] border border-[var(--critical)]/30 bg-[var(--critical)]/5 p-3 text-xs leading-5 text-[var(--critical)]">{error}</p> : null}
      <div className="flex justify-end gap-3 border-t border-[var(--border-soft)] pt-5"><button type="button" disabled={busy} onClick={onCancel} className={`${buttonClass} border border-[var(--border)] bg-[var(--panel)] text-[var(--text-2)] hover:text-[var(--text)]`}>{error ? 'Dismiss' : 'Cancel'}</button><button type="submit" disabled={busy || eligibleClusters.length === 0 || noCapabilities} className={`${buttonClass} bg-[var(--accent)] font-semibold text-[var(--accent-contrast)] hover:bg-[var(--accent-hover)]`}>{busy ? 'Saving desired state...' : 'Confirm'}</button></div>
    </form>
  </PanelLayout> : null}</Drawer>
}

function NumberField({ name, label, unit, value, min, disabled }: { name: string; label: string; unit?: string; value: number; min: number; disabled: boolean }) {
  return <label className="text-xs font-medium text-[var(--text-2)]">{label}{unit ? <span className="text-[var(--text-3)]"> ({unit})</span> : null}<input required name={name} type="number" min={min} step={1} defaultValue={value} disabled={disabled} className={fieldClass} /></label>
}

function collectExtensionNames(snapshot: ProjectStateSnapshot | null, cluster: Cluster): string[] {
  const names = new Set<string>()
  const actual = snapshot?.clusters.find((state) => state.cluster_id === cluster.id)?.actual_state
  for (const name of actual?.enabled_extensions ?? []) names.add(name)
  for (const name of Object.keys(actual?.extension_update_methods ?? {})) names.add(name)
  for (const name of cluster.enabled_extensions) names.delete(name)
  return [...names].sort()
}
