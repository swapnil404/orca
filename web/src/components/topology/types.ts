import type { XYPosition } from '@xyflow/react'

export const PALETTE_DRAG_TYPE = 'application/x-orca-topology-node'

export type PaletteNodeType = 'replica' | 'pgbouncer' | 'pgbackrest' | 'extension'

export type ProvisionRequest =
  | { type: 'replica'; clusterID: string }
  | { type: 'pgbouncer'; clusterID: string; poolMode: 'session' | 'transaction' | 'statement'; maxConnections: number }
  | { type: 'pgbackrest'; clusterID: string; repoPath: string; retentionFull: number; retentionDiff: number; fullIntervalSeconds: number; diffIntervalSeconds: number; incrIntervalSeconds: number }
  | { type: 'extension'; clusterID: string; extension: string }

export interface ProvisionDraft {
  id: string
  type: PaletteNodeType
  position: XYPosition
  stage: 'configuring' | 'submitting' | 'awaiting' | 'error'
  error?: string
  resourceID?: string
  clusterID?: string
  resourceName?: string
  expectedConfig?: string
  expectedRevision?: string
}
