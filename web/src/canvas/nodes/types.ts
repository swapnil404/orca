import type { Node } from '@xyflow/react'
import type { ActualBackup, ActualCluster, ActualPgBouncer, ActualReplica, Cluster, ProjectClusterState } from '../../types/resources'
import type { PaletteNodeType } from '../../components/topology/types'
import type { NodeStatus } from '../status'

export interface BaseNodeData extends Record<string, unknown> {
  label: string
  eyebrow: string
  detail: string
  status: NodeStatus
  cluster: Cluster
  state?: ProjectClusterState
}

export interface PrimaryNodeData extends BaseNodeData {
  kind: 'cluster'
  actual?: ActualCluster
}

export interface ReplicaNodeData extends BaseNodeData {
  kind: 'replica'
  replicaID: string
  actual?: ActualReplica
}

export interface PgBouncerNodeData extends BaseNodeData {
  kind: 'pgbouncer'
  actual?: ActualPgBouncer
}

export interface PgBackRestNodeData extends BaseNodeData {
  kind: 'pgbackrest'
  actual?: ActualBackup
}

export interface ExtensionNodeData extends BaseNodeData {
  kind: 'extension'
  extension: string
  version?: string
}

export interface PendingNodeData extends Record<string, unknown> {
  kind: 'pending'
  resourceType: PaletteNodeType
  label: string
  eyebrow: string
  detail: string
  stage: 'configuring' | 'submitting' | 'awaiting' | 'error'
  error?: string
  onDismiss: () => void
}

export type RealInfrastructureNodeData = PrimaryNodeData | ReplicaNodeData | PgBouncerNodeData | PgBackRestNodeData | ExtensionNodeData
export type InfrastructureNodeData = RealInfrastructureNodeData | PendingNodeData
export type InfrastructureNode = Node<InfrastructureNodeData, 'primary' | 'replica' | 'pgbouncer' | 'pgbackrest' | 'extension' | 'pending'>
export type PrimaryInfrastructureNode = Node<PrimaryNodeData, 'primary'>
export type PendingInfrastructureNode = Node<PendingNodeData, 'pending'>
