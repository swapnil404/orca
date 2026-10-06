import { useEffect, useState } from 'react'
import { restartProject } from '../api'
import { restartProgress, type RestartProgressRow, type RestartTarget } from '../lib/restart'
import { useTopologyStore } from '../store/topology'

export function useRestartProject(projectID: string) {
  const [submitting, setSubmitting] = useState(false)
  const [message, setMessage] = useState('')
  const [dialogOpen, setDialogOpen] = useState(false)
  const [confirmedRows, setConfirmedRows] = useState<RestartProgressRow[] | null>(null)
  const [targets, setTargets] = useState<RestartTarget[]>([])
  const [startedAt, setStartedAt] = useState<number | null>(null)
  const [now, setNow] = useState(Date.now)
  const snapshot = useTopologyStore((state) => state.snapshot)
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1_000)
    return () => window.clearInterval(timer)
  }, [])
  useEffect(() => {
    setTargets([]); setConfirmedRows(null); setStartedAt(null); setMessage(''); setDialogOpen(false)
    try {
      const saved = JSON.parse(window.sessionStorage.getItem(`orca.restart.${projectID}`) ?? 'null') as { targets?: RestartTarget[]; startedAt?: number; complete?: boolean } | null
      if (saved && Array.isArray(saved.targets) && typeof saved.startedAt === 'number' && saved.targets.every((target) => typeof target.id === 'string' && typeof target.name === 'string' && target.restart_generation !== undefined)) {
        setTargets(saved.targets); setStartedAt(saved.startedAt)
        if (saved.complete) setConfirmedRows(saved.targets.map((target) => ({ id: target.id, name: target.name, phase: 'complete', detail: 'Restart confirmed · healthy' })))
      }
    } catch { /* Tracking is optional when browser storage is unavailable. */ }
  }, [projectID])
  const rows = confirmedRows ?? targets.map((target) => restartProgress(target, snapshot?.project_id === projectID ? snapshot.clusters.find((state) => state.cluster_id === target.id) : undefined, now))
  const failed = message !== '' || rows.some((row) => row.phase === 'failed')
  const complete = rows.length > 0 && rows.every((row) => row.phase === 'complete')
  useEffect(() => {
    if (!complete || confirmedRows) return
    setConfirmedRows(rows)
    try { window.sessionStorage.setItem(`orca.restart.${projectID}`, JSON.stringify({ targets, startedAt, complete: true })) } catch { /* Keep the confirmed result in memory. */ }
  }, [complete, confirmedRows, rows, targets, startedAt, projectID])
  const restarting = submitting || rows.some((row) => row.phase === 'waiting' || row.phase === 'recovering')

  async function requestRestart() {
    setSubmitting(true)
    setMessage('')
    setConfirmedRows(null)
    setTargets([])
    setStartedAt(Date.now())
    setNow(Date.now())
    try {
      const clusters = await restartProject(projectID)
      setTargets(clusters)
      const requestedAt = Date.now()
      setStartedAt(requestedAt)
      try { window.sessionStorage.setItem(`orca.restart.${projectID}`, JSON.stringify({ targets: clusters, startedAt: requestedAt })) } catch { /* Live tracking continues without storage. */ }
      setNow(Date.now())
      setDialogOpen(false)
    } catch (cause) {
      setMessage(cause instanceof Error ? cause.message : 'Could not request the project restart.')
    } finally {
      setSubmitting(false)
    }
  }

  return { restarting, submitting, message, failed, complete, rows,
    elapsed: startedAt === null ? 0 : Math.max(0, Math.floor((now - startedAt) / 1_000)),
    dialogOpen, openDialog: () => setDialogOpen(true), closeDialog: () => setDialogOpen(false), requestRestart,
    dismiss: () => { setConfirmedRows(null); setTargets([]); setStartedAt(null); setMessage(''); try { window.sessionStorage.removeItem(`orca.restart.${projectID}`) } catch { /* No saved state to clear. */ } },
  }
}
