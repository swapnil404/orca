import type { InfrastructureNodeData } from '../canvas/nodes/types'
import type { Cluster, ProjectClusterState } from '../types/resources'

interface ServiceDetail {
  cluster: Cluster
  state?: ProjectClusterState
}

export type DetailSelection = InfrastructureNodeData | (ServiceDetail & {
  kind: 'pgbackrest' | 'extensions' | 'alerts'
})
