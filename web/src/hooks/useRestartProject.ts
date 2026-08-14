import { useState } from 'react'
import { restartProject } from '../api'

const successMessage = 'Restart request queued. Completion is not yet confirmed; offline agents will process the latest request after reconnecting.'

export function useRestartProject(projectID: string) {
  const [restarting, setRestarting] = useState(false)
  const [message, setMessage] = useState('')
  const [failed, setFailed] = useState(false)
  const [dialogOpen, setDialogOpen] = useState(false)

  async function requestRestart() {
    setRestarting(true)
    setMessage('')
    setFailed(false)
    try {
      await restartProject(projectID)
      setDialogOpen(false)
      setMessage(successMessage)
    } catch (cause) {
      setFailed(true)
      setMessage(cause instanceof Error ? cause.message : 'Could not request the project restart.')
    } finally {
      setRestarting(false)
    }
  }

  return {
    restarting,
    message,
    failed,
    dialogOpen,
    openDialog: () => setDialogOpen(true),
    closeDialog: () => setDialogOpen(false),
    requestRestart,
  }
}
