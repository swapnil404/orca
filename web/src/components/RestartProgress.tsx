import { Check, CheckCircle2, Circle, RotateCw, TriangleAlert, X } from 'lucide-react'
import type { RestartProgressRow } from '../lib/restart'

interface RestartProgressProps {
  rows: RestartProgressRow[]
  submitting: boolean
  message: string
  elapsed: number
  onDismiss: () => void
}

export function RestartProgress({ rows, submitting, message, elapsed, onDismiss }: RestartProgressProps) {
  if (!submitting && rows.length === 0 && !message) return null
  const done = rows.filter((row) => row.phase === 'complete').length
  const complete = rows.length > 0 && done === rows.length
  const failed = !!message || rows.some((row) => row.phase === 'failed')
  const active = submitting || rows.some((row) => row.phase === 'waiting' || row.phase === 'recovering')
  const acknowledged = rows.length > 0 && rows.every((row) => row.phase === 'recovering' || row.phase === 'complete')
  const tone = failed ? 'var(--critical)' : complete ? 'var(--healthy)' : 'var(--warning)'
  return <section aria-label="Restart progress" role={failed ? 'alert' : 'status'} className="mb-3 overflow-hidden rounded-[var(--radius-lg)] border bg-[var(--card)]" style={{ borderColor: `color-mix(in srgb, ${tone} 45%, transparent)` }}>
    <div className="flex items-center gap-3 px-4 py-3">
      <span className="grid h-10 w-10 shrink-0 place-items-center rounded-full" style={{ color: tone, background: `color-mix(in srgb, ${tone} 12%, transparent)` }}>{failed ? <TriangleAlert className="h-5 w-5" /> : complete ? <CheckCircle2 className="h-5 w-5" /> : <RotateCw className="h-5 w-5 motion-safe:animate-spin" />}</span>
      <div className="flex-1"><h2 className="text-sm font-semibold">{failed ? 'Restart needs attention' : complete ? 'Project restart complete' : submitting ? 'Sending restart request' : 'Restart in progress'}</h2><p className="mt-1 text-xs text-[var(--text-2)]">{message || (complete ? 'The agent confirmed the restart and services are healthy.' : 'Watching live agent reports for restart completion.')}</p></div>
      <span className="font-mono text-xs text-[var(--text-3)]">{complete ? `${done}/${rows.length} healthy` : `${elapsed}s`}</span>
      {!active && <button type="button" aria-label="Dismiss restart progress" onClick={onDismiss} className="rounded p-1.5 text-[var(--text-3)] hover:text-[var(--text)]"><X className="h-4 w-4" /></button>}
    </div>
    <div className="grid grid-cols-3 gap-2 border-y border-[var(--border)] px-4 py-3">{[{ label: 'Request saved', reached: rows.length > 0 }, { label: 'Agent confirmed', reached: acknowledged }, { label: 'Services healthy', reached: complete }].map((step) => <div key={step.label} className="flex items-center gap-2 text-[11px]" style={{ color: step.reached ? 'var(--healthy)' : 'var(--text-3)' }}>{step.reached ? <Check className="h-3.5 w-3.5" /> : <Circle className="h-3.5 w-3.5" />}{step.label}</div>)}</div>
    {rows.length > 0 && <div className="space-y-2 px-4 py-3">{rows.map((row) => <div key={row.id} className="flex items-center gap-3 text-xs"><span className="shrink-0" style={{ color: row.phase === 'complete' ? 'var(--healthy)' : row.phase === 'failed' ? 'var(--critical)' : 'var(--warning)' }}>{row.phase === 'complete' ? <CheckCircle2 className="h-4 w-4" /> : row.phase === 'failed' ? <TriangleAlert className="h-4 w-4" /> : <RotateCw className="h-4 w-4 motion-safe:animate-spin" />}</span><span className="min-w-20 font-medium">{row.name}</span><span className="text-[var(--text-2)]">{row.detail}</span></div>)}</div>}
    {active && elapsed >= 60 && <p className="px-4 pb-3 text-xs text-[var(--text-3)]">Still waiting for confirmation. Check that the host agent is running; the saved request remains queued.</p>}
    <div className="h-1 bg-[var(--border)]"><div className="h-full transition-[width] duration-500" style={{ width: `${rows.length ? done / rows.length * 100 : 0}%`, background: tone }} /></div>
  </section>
}
