import { applyNodeChanges, Background, Controls, ReactFlow, type NodeChange, type NodeMouseHandler, type OnNodeDrag, type ReactFlowInstance, type XYPosition } from '@xyflow/react'
import { useEffect, useRef, useState, type DragEvent } from 'react'
import { ApiError, addReplica, configurePgBackRest, enablePgBouncer, installExtension } from '../api'
import { NodePalette } from '../components/topology/NodePalette'
import { PALETTE_DRAG_TYPE, type PaletteNodeType, type ProvisionDraft, type ProvisionRequest } from '../components/topology/types'
import { PanelHost } from '../panels/PanelHost'
import { ProvisionPanel } from '../panels/ProvisionPanel'
import type { DetailSelection } from '../panels/types'
import type { Cluster, ProjectStateSnapshot } from '../types/resources'
import { ServiceCards, type ServiceKind } from './ServiceCards'
import { TopologyEdge } from './edges/TopologyEdge'
import { ExtensionNode } from './nodes/ExtensionNode'
import { PendingNode } from './nodes/PendingNode'
import { PgBackRestNode } from './nodes/PgBackRestNode'
import { PgBouncerNode } from './nodes/PgBouncerNode'
import { PrimaryNode } from './nodes/PrimaryNode'
import { ReplicaNode } from './nodes/ReplicaNode'
import type { InfrastructureNode } from './nodes/types'
import { buildCanvasTopology, extensionNodeID } from './topology'

const nodeTypes = { primary: PrimaryNode, replica: ReplicaNode, pgbouncer: PgBouncerNode, pgbackrest: PgBackRestNode, extension: ExtensionNode, pending: PendingNode }
const edgeTypes = { topology: TopologyEdge }

const draftLabels: Record<PaletteNodeType, { label: string; eyebrow: string; detail: string }> = {
  replica: { label: 'New replica', eyebrow: 'Streaming replica', detail: 'Configure before writing desired state' },
  pgbouncer: { label: 'PgBouncer', eyebrow: 'Connection pool', detail: 'Configure before writing desired state' },
  pgbackrest: { label: 'pgBackRest', eyebrow: 'Backup repository', detail: 'Configure repository and schedule' },
  extension: { label: 'Extension', eyebrow: 'PostgreSQL extension', detail: 'Choose a reported extension capability' },
}

interface CanvasViewProps {
  clusters: Cluster[]
  snapshot: ProjectStateSnapshot | null
  onClusterUpdated: (cluster: Cluster) => void
}

type SelectionTarget = { kind: 'node'; id: string } | { kind: ServiceKind; clusterID: string }

export function CanvasView({ clusters, snapshot, onClusterUpdated }: CanvasViewProps) {
  const [selection, setSelection] = useState<SelectionTarget | null>(null)
  const [drafts, setDrafts] = useState<ProvisionDraft[]>([])
  const [activeDraftID, setActiveDraftID] = useState<string | null>(null)
  const [flow, setFlow] = useState<ReactFlowInstance<InfrastructureNode> | null>(null)
  const [promotedPositions, setPromotedPositions] = useState<Record<string, XYPosition>>({})
  const [now, setNow] = useState(() => Date.now())
  const canvasRef = useRef<HTMLElement>(null)

  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 15_000)
    return () => window.clearInterval(timer)
  }, [])

  useEffect(() => {
    const awaiting = drafts.filter((draft) => draft.stage === 'awaiting' && reportReachedRevision(draft, snapshot))
    const failed = new Map(awaiting.flatMap((draft) => {
      const message = reconciliationFailure(draft, snapshot)
      return message ? [[draft.id, message] as const] : []
    }))
    const promoted = awaiting.filter((draft) => !failed.has(draft.id) && isApplied(draft, snapshot))
    if (promoted.length === 0 && failed.size === 0) return
    setPromotedPositions((current) => {
      const next = { ...current }
      for (const draft of promoted) if (draft.resourceID) next[draft.resourceID] = draft.position
      return next
    })
    setDrafts((current) => current
      .filter((draft) => !promoted.some((item) => item.id === draft.id))
      .map((draft) => failed.has(draft.id) ? { ...draft, stage: 'error', error: failed.get(draft.id) } : draft))
  }, [drafts, snapshot])

  const baseTopology = buildCanvasTopology(clusters, snapshot, now)
  const hiddenResourceIDs = new Set(drafts.flatMap((draft) => draft.stage === 'awaiting' && draft.resourceID ? [draft.resourceID] : []))
  const realNodes = baseTopology.nodes
    .filter((node) => !hiddenResourceIDs.has(node.id))
    .map((node) => promotedPositions[node.id] ? { ...node, position: promotedPositions[node.id] } : node)
  const draftNodes: InfrastructureNode[] = drafts.map((draft) => ({
    id: draft.id,
    type: 'pending',
    position: draft.position,
    draggable: true,
    connectable: false,
    data: { kind: 'pending', resourceType: draft.type, ...draftLabels[draft.type], stage: draft.stage, error: draft.error, onDismiss: () => dismissDraft(draft.id) },
  }))
  const generatedTopology = {
    nodes: [...realNodes, ...draftNodes],
    edges: baseTopology.edges.filter((edge) => !hiddenResourceIDs.has(edge.target) && !hiddenResourceIDs.has(edge.source)),
  }
  const [nodes, setNodes] = useState<InfrastructureNode[]>(generatedTopology.nodes)

  useEffect(() => {
    setNodes((current) => {
      const byID = new Map(current.map((node) => [node.id, node]))
      return generatedTopology.nodes.map((node) => {
        const existing = byID.get(node.id)
        return existing ? { ...existing, ...node, position: existing.position } : node
      })
    })
  }, [clusters, snapshot, now, drafts, promotedPositions])

  const topology = { nodes, edges: generatedTopology.edges }

  let selected: DetailSelection | null = null
  if (selection?.kind === 'node') {
    const data = topology.nodes.find((node) => node.id === selection.id)?.data
    selected = data && data.kind !== 'pending' ? data : null
  } else if (selection) {
    const cluster = clusters.find((candidate) => candidate.id === selection.clusterID)
    const state = snapshot?.clusters.find((candidate) => candidate.cluster_id === selection.clusterID)
    if (cluster && selection.kind === 'pgbouncer') {
      selected = topology.nodes.find((node) => node.id === `pgbouncer:${cluster.id}` && node.data.kind !== 'pending')?.data as DetailSelection | undefined ?? {
        kind: 'pgbouncer', label: 'PgBouncer', eyebrow: 'Connection pool', detail: 'Not enabled', status: 'unknown', cluster, state, actual: state?.actual_state?.pg_bouncer,
      }
    } else if (cluster && selection.kind !== 'pgbouncer') {
      selected = { kind: selection.kind, cluster, state }
    }
  }

  const activeDraft = drafts.find((draft) => draft.id === activeDraftID)

  function addDraft(type: PaletteNodeType, position: XYPosition) {
    const draft: ProvisionDraft = { id: `draft:${crypto.randomUUID()}`, type, position, stage: 'configuring' }
    setDrafts((current) => [...current, draft])
    setSelection(null)
    setActiveDraftID(draft.id)
  }

  function activatePaletteItem(type: PaletteNodeType) {
    const bounds = canvasRef.current?.getBoundingClientRect()
    const position = flow && bounds ? flow.screenToFlowPosition({ x: bounds.left + bounds.width / 2, y: bounds.top + bounds.height / 2 }) : { x: 300, y: 180 }
    addDraft(type, position)
  }

  function dropPaletteItem(event: DragEvent<HTMLElement>) {
    event.preventDefault()
    const type = event.dataTransfer.getData(PALETTE_DRAG_TYPE)
    if (!flow || !isPaletteNodeType(type)) return
    addDraft(type, flow.screenToFlowPosition({ x: event.clientX, y: event.clientY }))
  }

  function dismissDraft(draftID: string) {
    setDrafts((current) => current.filter((draft) => draft.id !== draftID))
    setActiveDraftID((current) => current === draftID ? null : current)
  }

  function changeNodes(changes: NodeChange<InfrastructureNode>[]) {
    setNodes((current) => applyNodeChanges(changes, current))
  }

  const finishMovingNode: OnNodeDrag<InfrastructureNode> = (_event, node) => {
    setDrafts((current) => current.map((draft) => draft.id === node.id ? { ...draft, position: node.position } : draft))
  }

  async function confirmProvision(request: ProvisionRequest) {
    if (!activeDraft) return
    const cluster = clusters.find((candidate) => candidate.id === request.clusterID)
    if (!cluster) return
    setDrafts((current) => current.map((draft) => draft.id === activeDraft.id ? { ...draft, stage: 'submitting', error: undefined } : draft))
    try {
      const updated = request.type === 'replica' ? await addReplica(cluster)
        : request.type === 'pgbouncer' ? await enablePgBouncer(cluster, { pool_mode: request.poolMode, max_connections: request.maxConnections, publish_address: request.publishAddress, publish_port: request.publishPort })
          : request.type === 'pgbackrest' ? await configurePgBackRest(cluster, { repo_path: request.repoPath, retention_full: request.retentionFull, retention_diff: request.retentionDiff, full_interval_seconds: request.fullIntervalSeconds, diff_interval_seconds: request.diffIntervalSeconds, incr_interval_seconds: request.incrIntervalSeconds })
            : await installExtension(cluster, request.extension)
      const newReplica = request.type === 'replica' ? updated.replicas.at(-1) : undefined
      const resourceID = request.type === 'replica' && newReplica ? `replica:${cluster.id}:${newReplica.id}`
        : request.type === 'pgbouncer' ? `pgbouncer:${cluster.id}`
          : request.type === 'pgbackrest' ? `pgbackrest:${cluster.id}`
            : request.type === 'extension' ? extensionNodeID(cluster.id, request.extension) : undefined
      if (!resourceID) throw new Error('The server did not return the provisioned resource identity.')
      onClusterUpdated(updated)
      const expectedConfig = request.type === 'pgbackrest' ? pgBackRestReconciliationState(cluster.id, request)
        : request.type === 'pgbouncer' ? `${request.poolMode}:${request.maxConnections}:${request.publishAddress}:${request.publishPort}` : undefined
      if (!updated.desired_revision) throw new Error('The server did not return a desired-state revision.')
      setDrafts((current) => current.map((draft) => draft.id === activeDraft.id ? { ...draft, stage: 'awaiting', clusterID: cluster.id, resourceID, resourceName: request.type === 'extension' ? request.extension : undefined, expectedConfig, expectedRevision: updated.desired_revision } : draft))
      setActiveDraftID(null)
    } catch (cause) {
      const message = cause instanceof ApiError ? cause.message : cause instanceof Error ? cause.message : 'Could not save desired state.'
      setDrafts((current) => current.map((draft) => draft.id === activeDraft.id ? { ...draft, stage: 'error', error: message } : draft))
    }
  }

  const selectNode: NodeMouseHandler<InfrastructureNode> = (_event, node) => {
    if (node.data.kind === 'pending') {
      setSelection(null)
      if (node.data.stage !== 'awaiting') setActiveDraftID(node.id)
    } else {
      setActiveDraftID(null)
      setSelection({ kind: 'node', id: node.id })
    }
  }
  const selectService = (kind: ServiceKind, clusterID: string) => { setActiveDraftID(null); setSelection({ kind, clusterID }) }

  return (
    <div className="flex flex-1 flex-col gap-3 lg:flex-row">
      <NodePalette onActivate={activatePaletteItem} />
      <div className="flex min-w-0 flex-1 flex-col gap-3">
        <section ref={canvasRef} onDragOver={(event) => { event.preventDefault(); event.dataTransfer.dropEffect = 'copy' }} onDrop={dropPaletteItem} className="relative min-h-[520px] flex-1 overflow-hidden rounded-[var(--radius-lg)] border border-[var(--border)] bg-[#0c0c0d] shadow-[0_24px_80px_rgba(0,0,0,0.2)]">
          <div className="pointer-events-none absolute left-4 top-4 z-10 flex items-center gap-2 rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--panel)] px-3 py-2 font-mono text-[10px] text-[var(--text-2)] sm:left-5 sm:top-5"><span className="h-1 w-1 rounded-full bg-[var(--accent)]" />Topology · {topology.nodes.length} nodes</div>
          <p className="pointer-events-none absolute right-5 top-5 z-10 hidden text-[11px] text-[var(--text-3)] sm:block">Drop a resource or select a node</p>
          <ReactFlow nodes={topology.nodes} edges={topology.edges} nodeTypes={nodeTypes} edgeTypes={edgeTypes} onInit={setFlow} onNodesChange={changeNodes} onNodeDragStop={finishMovingNode} onNodeClick={selectNode} onPaneClick={() => { setSelection(null); setActiveDraftID(null) }} nodesDraggable nodesConnectable={false} elementsSelectable fitView proOptions={{ hideAttribution: true }}>
            <Background color="#303034" gap={24} size={1} />
            <Controls showInteractive={false} className="!overflow-hidden !rounded-[var(--radius-lg)] !border-[var(--border)] !bg-[var(--panel)] !fill-[var(--text-2)] !shadow-xl [&_button]:!border-[var(--border-soft)] [&_button]:!bg-transparent [&_button:hover]:!bg-white/5" />
          </ReactFlow>
        </section>
        <ServiceCards clusters={clusters} states={snapshot?.clusters ?? []} now={now} onSelect={selectService} />
      </div>
      <PanelHost selected={selected} onClose={() => setSelection(null)} />
      <ProvisionPanel key={activeDraft?.id ?? 'closed'} type={activeDraft?.type ?? null} clusters={clusters} snapshot={snapshot} busy={activeDraft?.stage === 'submitting'} error={activeDraft?.error} onCancel={() => activeDraft && dismissDraft(activeDraft.id)} onClose={() => { if (activeDraft?.stage === 'configuring') dismissDraft(activeDraft.id); else setActiveDraftID(null) }} onConfirm={(request) => void confirmProvision(request)} />
    </div>
  )
}

function isPaletteNodeType(value: string): value is PaletteNodeType {
  return value === 'replica' || value === 'pgbouncer' || value === 'pgbackrest' || value === 'extension'
}

function isApplied(draft: ProvisionDraft, snapshot: ProjectStateSnapshot | null): boolean {
  if (!draft.clusterID || !draft.resourceID) return false
  const state = snapshot?.clusters.find((candidate) => candidate.cluster_id === draft.clusterID)
  if (!state?.actual_state || state.stale) return false
  if (draft.type === 'replica') {
    const prefix = `replica:${draft.clusterID}:`
    const replicaID = draft.resourceID.startsWith(prefix) ? draft.resourceID.slice(prefix.length) : ''
    return state.actual_state.replicas?.some((replica) => replica.id === replicaID && replica.status === 'running' && replica.standby_connected === true && replica.streaming_state === 'streaming') ?? false
  }
  if (draft.type === 'pgbouncer') {
    const pooler = state.actual_state.pg_bouncer
    if (pooler?.status !== 'running' || !pooler.config) return false
    const values = Object.fromEntries(pooler.config.split('\n').map((line) => line.split('=').map((part) => part.trim())).filter((parts) => parts.length === 2))
    return `${values.pool_mode}:${values.max_client_conn}` === draft.expectedConfig
  }
  if (draft.type === 'pgbackrest') return state.actual_state.backup?.config === draft.expectedConfig
  return state.actual_state.enabled_extensions?.includes(draft.resourceName ?? '') ?? false
}

function reportReachedRevision(draft: ProvisionDraft, snapshot: ProjectStateSnapshot | null): boolean {
  if (!draft.clusterID || !draft.expectedRevision) return false
  const revision = snapshot?.clusters.find((candidate) => candidate.cluster_id === draft.clusterID)?.desired_state_revision
  if (!revision) return false
  const desiredRevision = revision.split(':', 1)[0]
  try {
    return BigInt(desiredRevision) >= BigInt(draft.expectedRevision)
  } catch {
    return desiredRevision === draft.expectedRevision
  }
}

function reconciliationFailure(draft: ProvisionDraft, snapshot: ProjectStateSnapshot | null): string | undefined {
  if (!draft.clusterID) return undefined
  const results = snapshot?.clusters.find((candidate) => candidate.cluster_id === draft.clusterID)?.reconciliation_results ?? []
  const actions: Record<PaletteNodeType, string[]> = {
    replica: ['create_replica'],
    pgbouncer: ['create_pgbouncer', 'update_pgbouncer'],
    pgbackrest: ['create_pgbackrest', 'update_pgbackrest'],
    extension: ['update_extensions'],
  }
  const failed = results.find((result) => result.status === 'failed' && actions[draft.type].includes(result.action))
    ?? results.find((result) => result.status === 'failed')
  if (!failed) return undefined
  return failed.error || `Agent reported ${failed.action.replaceAll('_', ' ')} failed.`
}

function pgBackRestReconciliationState(clusterID: string, request: Extract<ProvisionRequest, { type: 'pgbackrest' }>): string {
  return `[global]\nrepo1-path=${request.repoPath}\nrepo1-retention-full=${request.retentionFull}\nrepo1-retention-diff=${request.retentionDiff}\n\n[${clusterID}]\npg1-path=/var/orca/data/${clusterID}/primary\n\n[orca-schedule]\nfull=${request.fullIntervalSeconds}\ndiff=${request.diffIntervalSeconds}\nincr=${request.incrIntervalSeconds}\n\n[orca-storage]\nrepo-bind=${request.repoPath}\n`
}
