import { AlertTriangle, RotateCw, X } from 'lucide-react'
import { useEffect, useRef } from 'react'

interface RestartProjectDialogProps {
  open: boolean
  clusterCount: number
  restarting: boolean
  onCancel: () => void
  onConfirm: () => void
}

export function RestartProjectDialog({ open, clusterCount, restarting, onCancel, onConfirm }: RestartProjectDialogProps) {
  const cancelRef = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    if (!open) return
    cancelRef.current?.focus()
    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === 'Escape' && !restarting) onCancel()
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [onCancel, open, restarting])

  if (!open) return null

  return <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/65 px-4 backdrop-blur-sm" onMouseDown={() => { if (!restarting) onCancel() }}><section role="dialog" aria-modal="true" aria-labelledby="restart-project-title" aria-describedby="restart-project-description" onMouseDown={(event) => event.stopPropagation()} className="relative w-full max-w-lg overflow-hidden rounded-[var(--radius-lg)] border border-[var(--warning)]/35 bg-[color:rgba(17,17,18,0.98)] shadow-[0_28px_90px_rgba(0,0,0,0.7)]"><div aria-hidden="true" className="absolute inset-x-0 top-0 h-px bg-gradient-to-r from-transparent via-[var(--warning)] to-transparent" /><div className="flex items-start gap-4 border-b border-[var(--border)] px-5 py-5 sm:px-6"><span className="grid h-10 w-10 shrink-0 place-items-center rounded-full border border-[var(--warning)]/30 bg-[var(--warning)]/10 text-[var(--warning)]"><AlertTriangle className="h-5 w-5" /></span><div className="min-w-0 flex-1"><h2 id="restart-project-title" className="text-lg font-semibold tracking-[-0.02em]">Restart all project containers?</h2><p id="restart-project-description" className="mt-1.5 text-sm leading-6 text-[var(--text-2)]">This queues a restart for {clusterCount} cluster{clusterCount === 1 ? '' : 's'} across every connected host.</p></div><button type="button" aria-label="Close restart confirmation" disabled={restarting} onClick={onCancel} className="rounded-[var(--radius-sm)] p-1.5 text-[var(--text-3)] hover:bg-white/[0.06] hover:text-[var(--text)] disabled:opacity-40"><X className="h-4 w-4" /></button></div><div className="space-y-4 px-5 py-5 sm:px-6"><div className="rounded-[var(--radius-md)] border border-[var(--critical)]/25 bg-[var(--critical)]/5 px-4 py-3"><p className="text-sm font-medium text-[var(--critical)]">Active database connections will be interrupted</p><p className="mt-1 text-xs leading-5 text-[var(--text-2)]">In-flight transactions may roll back. Applications must reconnect after PostgreSQL and PgBouncer return.</p></div><div className="grid grid-cols-3 gap-2 text-center"><Impact label="Primaries" /><Impact label="Replicas" /><Impact label="PgBouncer" /></div><p className="text-xs leading-5 text-[var(--text-3)]">The request is durable and latest-request-wins. Offline agents process it after reconnecting; completion is reported asynchronously.</p></div><div className="flex flex-col-reverse gap-2 border-t border-[var(--border)] bg-black/10 px-5 py-4 sm:flex-row sm:justify-end sm:px-6"><button ref={cancelRef} type="button" disabled={restarting} onClick={onCancel} className="rounded-[var(--radius-md)] border border-[var(--border)] px-4 py-2.5 text-sm font-medium text-[var(--text-2)] hover:border-[var(--text-3)] hover:text-[var(--text)] disabled:opacity-40">Cancel</button><button type="button" disabled={restarting} onClick={onConfirm} className="inline-flex items-center justify-center gap-2 rounded-[var(--radius-md)] bg-[var(--warning)] px-4 py-2.5 text-sm font-semibold text-black hover:brightness-110 disabled:cursor-not-allowed disabled:opacity-50"><RotateCw className={`h-4 w-4 ${restarting ? 'animate-spin' : ''}`} />{restarting ? 'Queuing restart...' : 'Restart project'}</button></div></section></div>
}

function Impact({ label }: { label: string }) {
  return <div className="rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--panel)] px-2 py-2.5 font-mono text-[10px] uppercase tracking-wide text-[var(--text-2)]">{label}</div>
}
