import { useEffect } from 'react'
import { connectProjectEvents } from '../api'
import { useTopologyStore } from '../store/topology'

export function useProjectEvents(projectID: string): void {
  const replaceSnapshot = useTopologyStore((state) => state.replaceSnapshot)
  const reset = useTopologyStore((state) => state.reset)

  useEffect(() => {
    reset()
    const connection = connectProjectEvents({
      projectID,
      onSnapshot: replaceSnapshot,
    })
    return () => {
      connection.close()
      reset()
    }
  }, [projectID, replaceSnapshot, reset])
}
