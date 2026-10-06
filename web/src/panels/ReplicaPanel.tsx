import { useEffect, useState } from 'react'
import { LoaderCircle, Trash2 } from 'lucide-react'
import { removeReplica } from '../api'
import type { Cluster } from '../types/resources'
import type { ReplicaNodeData } from '../canvas/nodes/types'
import { PanelLayout, StateRow } from './PanelLayout'

interface ReplicaPanelProps {
  resource: ReplicaNodeData
  onClose: () => void
  onRemoved: (cluster: Cluster, replicaID: string) => void
}

export function ReplicaPanel({ resource, onClose, onRemoved }: ReplicaPanelProps) {
  const [confirming, setConfirming] = useState(false)
  const [removing, setRemoving] = useState(false)
  const [error, setError] = useState('')
  useEffect(() => { setConfirming(false); setError('') }, [resource.replicaID])
  async function remove() {
    setRemoving(true); setError('')
    try {
      const cluster = await removeReplica(resource.cluster.id, resource.replicaID)
      onRemoved(cluster, resource.replicaID)
      onClose()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not remove this replica.')
    } finally { setRemoving(false) }
  }
  return (
    <PanelLayout title={`Replica ${resource.replicaID}`} eyebrow="Replica state" onClose={onClose}>
      <dl>
        <StateRow label="Desired" value="Present" />
        <StateRow label="Container" value={resource.actual?.status ?? 'Unknown'} />
        <StateRow label="Streaming" value={resource.actual?.streaming_state ?? 'Unknown'} />
        <StateRow label="Standby connected" value={resource.actual?.standby_connected === undefined ? 'Unknown' : resource.actual.standby_connected ? 'Yes' : 'No'} />
        <StateRow label="Replication lag" value={resource.actual?.replication_lag_bytes === undefined ? 'Unknown' : `${resource.actual.replication_lag_bytes} bytes`} />
        <StateRow label="Lag status" value={resource.actual?.replication_lag_status ?? 'Unknown'} />
      </dl>
      <section className="mt-6 rounded-[var(--radius-md)] border border-[var(--critical)]/30 bg-[var(--critical)]/5 p-4">
        <h3 className="text-sm font-medium">Remove this replica</h3>
        <p className="mt-2 text-xs leading-5 text-[var(--text-2)]">The agent will remove this replica's container, data volume, and replication slot. The primary and other replicas stay in place.</p>
        {error && <p role="alert" className="mt-3 text-xs text-[var(--critical)]">{error}</p>}
        {confirming ? <div className="mt-4"><p className="mb-3 text-xs text-[var(--critical)]">Remove replica {resource.replicaID}?</p><div className="flex gap-2"><button type="button" disabled={removing} onClick={() => setConfirming(false)} className="rounded-md border border-[var(--border)] px-3 py-2 text-xs disabled:opacity-50">Cancel</button><button type="button" disabled={removing} onClick={() => void remove()} className="inline-flex items-center gap-2 rounded-md bg-[var(--critical)] px-3 py-2 text-xs font-medium text-white disabled:opacity-50">{removing ? <LoaderCircle className="h-3.5 w-3.5 animate-spin" /> : <Trash2 className="h-3.5 w-3.5" />}{removing ? 'Removing...' : 'Confirm removal'}</button></div></div> : <button type="button" onClick={() => setConfirming(true)} className="mt-4 inline-flex items-center gap-2 rounded-md border border-[var(--critical)]/40 px-3 py-2 text-xs text-[var(--critical)]"><Trash2 className="h-3.5 w-3.5" />Remove replica</button>}
      </section>
    </PanelLayout>
  )
}
