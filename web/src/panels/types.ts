import type { RealInfrastructureNodeData } from '../canvas/nodes/types'
import type { Cluster, ProjectClusterState } from '../types/resources'

interface ServiceDetail {
  cluster: Cluster
  state?: ProjectClusterState
}

export type DetailSelection = RealInfrastructureNodeData | (ServiceDetail & {
  kind: 'pgbackrest' | 'extensions' | 'alerts'
})
