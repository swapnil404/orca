import type { Node } from '@xyflow/react'
import type { ActualCluster, ActualPgBouncer, ActualReplica, Cluster, ProjectClusterState } from '../../types/resources'
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

export type InfrastructureNodeData = PrimaryNodeData | ReplicaNodeData | PgBouncerNodeData
export type InfrastructureNode = Node<InfrastructureNodeData, 'primary' | 'replica' | 'pgbouncer'>
