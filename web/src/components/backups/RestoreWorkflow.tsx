import { AlertTriangle, CheckCircle2, Clock3, DatabaseZap, LoaderCircle, RotateCcw, ShieldAlert, XCircle } from 'lucide-react'
import { useEffect, useState, type FormEvent } from 'react'
import { ApiError, cancelRestoreOperation, confirmRestoreOperation, createRestoreOperation, requestRestoreOperationAction, type ConfirmedRestoreOperationAction } from '../../api'
import type { Cluster, CreateRestoreOperationInput, RestoreOperation, RestoreOperationMode, RestoreOperationStatus } from '../../types/resources'

interface RestoreWorkflowProps {
  cluster: Cluster
  operations: RestoreOperation[]
  onOperation: (operation: RestoreOperation) => void
}

const fieldClass = 'mt-2 w-full rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--panel)] px-3 py-2.5 font-mono text-sm text-[var(--text)] outline-none hover:border-[var(--text-3)] focus:border-[var(--accent)] focus:ring-2 focus:ring-[var(--accent-soft)] disabled:cursor-not-allowed disabled:opacity-50'
const primaryButtonClass = 'inline-flex items-center justify-center rounded-[var(--radius-md)] bg-[var(--accent)] px-4 py-2.5 text-sm font-semibold text-[var(--accent-contrast)] hover:bg-[var(--accent-hover)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:ring-offset-2 focus:ring-offset-[var(--card)] disabled:cursor-not-allowed disabled:opacity-50'
const secondaryButtonClass = 'inline-flex items-center justify-center rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--panel)] px-3.5 py-2.5 text-sm font-medium text-[var(--text-2)] hover:border-[var(--text-3)] hover:text-[var(--text)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)] disabled:cursor-not-allowed disabled:opacity-50'
const activeStatuses: RestoreOperationStatus[] = ['pending', 'ready', 'running']

function newIdempotencyKey(): string {
  return `orca-restore-${crypto.randomUUID()}`
}

function formatBytes(value: number | undefined): string {
  if (!value) return '0 B'
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']
  const unit = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1)
  return `${(value / 1024 ** unit).toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`
}

function label(value: string): string {
  return value.replaceAll('_', ' ')
}

function operationTone(status: RestoreOperationStatus): string {
  if (status === 'failed') return 'border-[var(--critical)]/30 text-[var(--critical)]'
  if (status === 'succeeded' || status === 'finalized') return 'border-[var(--healthy)]/30 text-[var(--healthy)]'
  if (status === 'running' || status === 'ready') return 'border-[var(--warning)]/30 text-[var(--warning)]'
  return 'border-[var(--border)] text-[var(--text-3)]'
}

export function RestoreWorkflow({ cluster, operations, onOperation }: RestoreWorkflowProps) {
  const [selectedOperationID, setSelectedOperationID] = useState(() => operations[0]?.id ?? '')
  const [mode, setMode] = useState<RestoreOperationMode>('clone')
  const [recoveryTime, setRecoveryTime] = useState('')
  const [cloneName, setCloneName] = useState(`${cluster.name}-restore`)
  const [confirmation, setConfirmation] = useState('')
  const [confirmedAction, setConfirmedAction] = useState<ConfirmedRestoreOperationAction | null>(null)
  const [actionConfirmation, setActionConfirmation] = useState('')
  const [idempotencyKey, setIdempotencyKey] = useState(newIdempotencyKey)
  const [busyAction, setBusyAction] = useState<'create' | 'confirm' | 'cancel' | ConfirmedRestoreOperationAction | null>(null)
  const [message, setMessage] = useState('')
  const [failed, setFailed] = useState(false)
  const operation = operations.find((item) => item.id === selectedOperationID)
  const conflict = operations.find((item) => activeStatuses.includes(item.status))
  const maxDateTime = new Date(Date.now() - new Date().getTimezoneOffset() * 60_000).toISOString().slice(0, 16)

  useEffect(() => {
    if (selectedOperationID && !operations.some((item) => item.id === selectedOperationID)) setSelectedOperationID(operations[0]?.id ?? '')
  }, [operations, selectedOperationID])

  function updateDraft(changes: () => void) {
    changes()
    setIdempotencyKey(newIdempotencyKey())
    setMessage('')
    setFailed(false)
  }

  async function create(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!recoveryTime || conflict) return
    const targetTime = new Date(recoveryTime)
    if (Number.isNaN(targetTime.getTime())) {
      setFailed(true)
      setMessage('Enter a valid local recovery date and time.')
      return
    }
    const input: CreateRestoreOperationInput = mode === 'clone'
      ? { mode, target_time: targetTime.toISOString(), target_cluster_name: cloneName.trim() }
      : { mode, target_time: targetTime.toISOString() }
    setBusyAction('create')
    setFailed(false)
    setMessage('')
    try {
      const created = await createRestoreOperation(cluster.id, input, idempotencyKey)
      onOperation(created)
      setSelectedOperationID(created.id)
      setIdempotencyKey(newIdempotencyKey())
      setMessage('Preflight requested. Waiting for the agent report.')
    } catch (cause) {
      setFailed(true)
      setMessage(cause instanceof ApiError ? cause.message : 'Could not request restore preflight.')
    } finally {
      setBusyAction(null)
    }
  }

  async function confirm() {
    if (!operation) return
    setBusyAction('confirm')
    setFailed(false)
    setMessage('')
    try {
      const updated = await confirmRestoreOperation(operation.id, confirmation)
      onOperation(updated)
      setMessage('Execution requested. Waiting for the agent to begin.')
    } catch (cause) {
      setFailed(true)
      setMessage(cause instanceof ApiError ? cause.message : 'Could not confirm restore execution.')
    } finally {
      setBusyAction(null)
    }
  }

  async function cancel() {
    if (!operation) return
    setBusyAction('cancel')
    setFailed(false)
    setMessage('')
    try {
      const updated = await cancelRestoreOperation(operation.id)
      onOperation(updated)
      setMessage('Cancel requested. Waiting for the agent report.')
    } catch (cause) {
      setFailed(true)
      setMessage(cause instanceof ApiError ? cause.message : 'Could not request cancel.')
    } finally {
      setBusyAction(null)
    }
  }

  async function requestConfirmedAction() {
    if (!operation || !confirmedAction) return
    setBusyAction(confirmedAction)
    setFailed(false)
    setMessage('')
    try {
      const updated = await requestRestoreOperationAction(operation.id, confirmedAction, actionConfirmation)
      onOperation(updated)
      setMessage(`${label(confirmedAction)} requested. Waiting for the agent report.`)
      setConfirmedAction(null)
      setActionConfirmation('')
    } catch (cause) {
      setFailed(true)
      setMessage(cause instanceof ApiError ? cause.message : `Could not request ${label(confirmedAction)}.`)
    } finally {
      setBusyAction(null)
    }
  }

  const expectedConfirmation = operation?.mode === 'clone' ? operation.target_cluster_name ?? '' : operation?.source_cluster_id ?? ''
  const report = operation?.report
  const canConfirm = operation?.status === 'ready' && operation.intent === 'preflight'
  const canCancel = operation?.intent !== 'cancel' && (operation?.status === 'pending' || operation?.status === 'ready' || (operation?.status === 'running' && Boolean(report?.cancellable) && !report?.destructive_started))
  const canRollback = operation?.intent !== 'rollback' && operation?.intent !== 'finalize' && (operation?.status === 'failed' || (operation?.mode === 'in_place' && operation?.status === 'succeeded' && Boolean(report?.rollback_available)))
  const canFinalize = (operation?.status === 'succeeded' || operation?.status === 'rolled_back') && operation.intent !== 'finalize'

  return (
    <section className="rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--card)] p-5 sm:p-6">
      <div className="mb-6 flex items-start gap-3"><span className="grid h-8 w-8 shrink-0 place-items-center rounded-full border border-[var(--warning)]/30 bg-[var(--warning)]/10"><ShieldAlert className="h-4 w-4 text-[var(--warning)]" /></span><div><h2 className="font-medium">Point-in-time restore</h2><p className="mt-1 text-xs leading-5 text-[var(--text-3)]">Preflight, confirm, and monitor a durable recovery operation.</p></div></div>

      {operations.length > 0 && <div className="mb-6"><label className="block text-xs font-medium text-[var(--text-2)]">Restore operation<select value={selectedOperationID} onChange={(event) => { setSelectedOperationID(event.target.value); setConfirmation(''); setConfirmedAction(null); setActionConfirmation(''); setMessage(''); setFailed(false) }} className={fieldClass}><option value="">Start a new restore</option>{operations.map((item) => <option key={item.id} value={item.id}>{new Date(item.created_at).toLocaleString()} · {label(item.mode)} · {label(item.status)}</option>)}</select></label></div>}

      {!operation ? <form onSubmit={create} className="space-y-5">
        <fieldset disabled={busyAction !== null || Boolean(conflict)}><legend className="text-xs font-medium text-[var(--text-2)]">Restore mode</legend><div className="mt-2 grid gap-3 sm:grid-cols-2"><ModeOption checked={mode === 'clone'} title="Same-host clone" description="Create an isolated cluster beside the source." onChange={() => updateDraft(() => setMode('clone'))} /><ModeOption checked={mode === 'in_place'} title="In-place" description="Replace the source cluster's current data." onChange={() => updateDraft(() => setMode('in_place'))} /></div></fieldset>
        <div className="rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--panel)] p-4 text-xs leading-5 text-[var(--text-2)]"><p><strong className="text-[var(--text)]">Same-host only.</strong> Clone storage and recovery run on the source agent host; this workflow cannot restore to another host.</p><p className="mt-2"><strong className="text-[var(--text)]">Matching PostgreSQL major required.</strong> Preflight verifies the repository, source, and generated clone target use the same major version before execution is allowed.</p></div>
        <label className="block text-xs font-medium text-[var(--text-2)]">Recovery date and time <span className="font-normal text-[var(--text-3)]">(your local timezone)</span><input required type="datetime-local" max={maxDateTime} value={recoveryTime} disabled={busyAction !== null || Boolean(conflict)} onChange={(event) => updateDraft(() => setRecoveryTime(event.target.value))} className={fieldClass} /></label>
        {mode === 'clone' && <label className="block text-xs font-medium text-[var(--text-2)]">Clone cluster name<input required value={cloneName} disabled={busyAction !== null || Boolean(conflict)} onChange={(event) => updateDraft(() => setCloneName(event.target.value))} className={fieldClass} /></label>}
        {conflict && <p role="alert" className="rounded-[var(--radius-md)] border border-[var(--warning)]/30 bg-[var(--warning)]/5 px-3 py-2 text-xs leading-5 text-[var(--warning)]">This cluster already has a {label(conflict.status)} restore. Select that operation above to monitor or manage it.</p>}
        <div className="flex justify-end border-t border-[var(--border)] pt-5"><button disabled={busyAction !== null || Boolean(conflict) || !recoveryTime || (mode === 'clone' && !cloneName.trim())} className={primaryButtonClass}>{busyAction === 'create' ? <><LoaderCircle className="mr-2 h-4 w-4 animate-spin" />Requesting preflight...</> : 'Create and run preflight'}</button></div>
      </form> : <div className="space-y-5">
        <OperationHeader operation={operation} />
        <div className="grid gap-3 sm:grid-cols-2"><Detail label="Recovery target" value={new Date(operation.target_time).toLocaleString()} /><Detail label="Destination" value={operation.mode === 'clone' ? `${operation.target_cluster_name} · same host` : `${cluster.name} · in place`} /><Detail label="Agent phase" value={report ? label(report.phase) : 'Awaiting first report'} /><Detail label="PostgreSQL version" value={report?.postgres_version || 'Pending preflight'} /></div>
        {report && (report.backup_label || report.recovery_earliest || (report.required_bytes ?? 0) > 0) && <div aria-label="Preflight metadata" className="rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--panel)] p-4"><p className="mb-3 font-mono text-[10px] uppercase tracking-[0.14em] text-[var(--text-3)]">Preflight metadata</p><dl className="grid gap-x-4 gap-y-3 text-xs sm:grid-cols-2"><Metadata label="Backup set" value={report.backup_label || 'Unknown'} /><Metadata label="Recoverable window" value={report.recovery_earliest && report.recovery_latest ? `${new Date(report.recovery_earliest).toLocaleString()} – ${new Date(report.recovery_latest).toLocaleString()}` : 'Unknown'} /><Metadata label="Required space" value={formatBytes(report.required_bytes)} /><Metadata label="Available space" value={formatBytes(report.available_bytes)} /></dl></div>}
        {report?.error && <div role="alert" className="flex gap-3 rounded-[var(--radius-md)] border border-[var(--critical)]/30 bg-[var(--critical)]/5 p-3 text-xs leading-5 text-[var(--critical)]"><XCircle className="mt-0.5 h-4 w-4 shrink-0" /><div><p className="font-medium">{report.error_code ? label(report.error_code) : 'Restore failed'}</p><p className="mt-1 text-[var(--text-2)]">{report.error}</p></div></div>}
        {canConfirm && <div className="rounded-[var(--radius-md)] border border-[var(--warning)]/30 bg-[var(--warning)]/5 p-4"><div className="flex gap-3"><AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-[var(--warning)]" /><div><p className="text-sm font-medium">Explicit confirmation required</p><p className="mt-1 text-xs leading-5 text-[var(--text-2)]">{operation.mode === 'in_place' ? 'In-place recovery stops PostgreSQL and replaces source data. Data after the recovery target will be unavailable.' : 'The clone will consume host storage and is created with the source PostgreSQL major version.'}</p></div></div><label className="mt-4 block text-xs font-medium text-[var(--text-2)]">Type <code className="select-all text-[var(--text)]">{expectedConfirmation}</code> to execute<input value={confirmation} disabled={busyAction !== null} autoComplete="off" spellCheck={false} onChange={(event) => setConfirmation(event.target.value)} className={fieldClass} /></label><button type="button" disabled={busyAction !== null || confirmation !== expectedConfirmation} onClick={confirm} className={`${primaryButtonClass} mt-4 w-full sm:w-auto`}>{busyAction === 'confirm' ? 'Requesting execution...' : 'Execute restore'}</button></div>}
        <div className="flex flex-wrap items-center gap-2 border-t border-[var(--border)] pt-5"><button type="button" disabled={busyAction !== null || !canCancel} onClick={() => void cancel()} className={secondaryButtonClass}>Cancel</button><button type="button" disabled={busyAction !== null || !canRollback} onClick={() => { setConfirmedAction('rollback'); setActionConfirmation(''); setMessage(''); setFailed(false) }} className={secondaryButtonClass}><RotateCcw className="mr-2 h-4 w-4" />Rollback</button><button type="button" disabled={busyAction !== null || !canFinalize} onClick={() => { setConfirmedAction('finalize'); setActionConfirmation(''); setMessage(''); setFailed(false) }} className={secondaryButtonClass}>Finalize</button>{!activeStatuses.includes(operation.status) && <button type="button" disabled={Boolean(conflict)} onClick={() => { setSelectedOperationID(''); setConfirmation(''); setConfirmedAction(null); setActionConfirmation(''); setMessage(''); setFailed(false) }} className="ml-auto text-xs font-medium text-[var(--accent)] hover:underline disabled:cursor-not-allowed disabled:opacity-50">Start another restore</button>}</div>
        {confirmedAction && <section aria-labelledby="restore-action-confirmation-title" className="rounded-[var(--radius-md)] border border-[var(--critical)]/30 bg-[var(--critical)]/5 p-4"><div className="flex gap-3"><AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-[var(--critical)]" /><div><h3 id="restore-action-confirmation-title" className="text-sm font-medium capitalize">Confirm {confirmedAction}</h3><p className="mt-1 text-xs leading-5 text-[var(--text-2)]">{confirmedAction === 'rollback' ? 'Rollback replaces the recovered data with the retained pre-restore data. Changes made to the recovered data will be lost.' : 'Finalize permanently removes rollback data. The retained pre-restore data will no longer be available through this operation.'}</p></div></div><label className="mt-4 block text-xs font-medium text-[var(--text-2)]">Type <code className="select-all text-[var(--text)]">{expectedConfirmation}</code> to confirm<input value={actionConfirmation} disabled={busyAction !== null} autoComplete="off" spellCheck={false} onChange={(event) => setActionConfirmation(event.target.value)} className={fieldClass} /></label><div className="mt-4 flex flex-wrap gap-2"><button type="button" disabled={busyAction !== null || actionConfirmation !== expectedConfirmation} onClick={() => void requestConfirmedAction()} className={primaryButtonClass}>{busyAction === confirmedAction ? `Requesting ${label(confirmedAction)}...` : `Confirm ${confirmedAction}`}</button><button type="button" disabled={busyAction !== null} onClick={() => { setConfirmedAction(null); setActionConfirmation('') }} className={secondaryButtonClass}>Keep current data</button></div></section>}
      </div>}
      {message && <p role={failed ? 'alert' : 'status'} aria-live="polite" className={`mt-4 rounded-[var(--radius-md)] border px-3 py-2 text-xs leading-5 ${failed ? 'border-[var(--critical)]/30 bg-[var(--critical)]/5 text-[var(--critical)]' : 'border-[var(--border)] bg-[var(--panel)] text-[var(--text-2)]'}`}>{message}</p>}
    </section>
  )
}

function ModeOption({ checked, title, description, onChange }: { checked: boolean; title: string; description: string; onChange: () => void }) {
  return <label className={`cursor-pointer rounded-[var(--radius-md)] border p-3 transition-colors ${checked ? 'border-[var(--accent)] bg-[var(--accent-soft)]' : 'border-[var(--border)] bg-[var(--panel)] hover:border-[var(--text-3)]'}`}><input type="radio" name="restore-mode" checked={checked} onChange={onChange} className="sr-only" /><span className="block text-sm font-medium text-[var(--text)]">{title}</span><span className="mt-1 block text-xs leading-5 text-[var(--text-3)]">{description}</span></label>
}

function OperationHeader({ operation }: { operation: RestoreOperation }) {
  const active = activeStatuses.includes(operation.status)
  return <div role={operation.status === 'failed' ? 'alert' : 'status'} aria-live="polite" className="flex flex-wrap items-center justify-between gap-3 rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--panel)] p-4"><div className="flex min-w-0 items-center gap-3">{active ? <Clock3 className="h-4 w-4 shrink-0 text-[var(--warning)]" /> : operation.status === 'failed' ? <XCircle className="h-4 w-4 shrink-0 text-[var(--critical)]" /> : operation.status === 'succeeded' || operation.status === 'finalized' ? <CheckCircle2 className="h-4 w-4 shrink-0 text-[var(--healthy)]" /> : <DatabaseZap className="h-4 w-4 shrink-0 text-[var(--text-3)]" />}<div className="min-w-0"><p className="text-sm font-medium capitalize">{label(operation.mode)} restore</p><p className="mt-1 truncate font-mono text-[10px] text-[var(--text-3)]">{operation.id}</p></div></div><span className={`rounded-full border px-2.5 py-1 font-mono text-[10px] uppercase tracking-wide ${operationTone(operation.status)}`}>{label(operation.status)}</span></div>
}

function Detail({ label: detailLabel, value }: { label: string; value: string }) {
  return <div className="rounded-[var(--radius-md)] border border-[var(--border-soft)] bg-[var(--panel)] p-3"><p className="font-mono text-[10px] uppercase tracking-wide text-[var(--text-3)]">{detailLabel}</p><p className="mt-1.5 break-words text-xs text-[var(--text)]">{value}</p></div>
}

function Metadata({ label: metadataLabel, value }: { label: string; value: string }) {
  return <div><dt className="text-[var(--text-3)]">{metadataLabel}</dt><dd className="mt-1 break-words font-mono text-[var(--text)]">{value}</dd></div>
}
