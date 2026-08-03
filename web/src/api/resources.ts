import type { BackupJob, Cluster, ClusterInput, CreateRestoreOperationInput, HostRegistration, PgBouncerConfig, PgHbaRule, Project, ProjectHost, ProjectTopology, RestoreOperation } from '../types/resources'
import { apiRequest } from './client'

const encode = encodeURIComponent

export function listProjects(): Promise<Project[]> {
  return apiRequest('/projects')
}

export function createProject(name: string, organizationID: string): Promise<Project> {
  return apiRequest('/projects', { method: 'POST', body: JSON.stringify({ name, organization_id: organizationID }) })
}

export function getProject(projectID: string): Promise<Project> {
  return apiRequest(`/projects/${encode(projectID)}`)
}

export function updateProject(projectID: string, name: string): Promise<Project> {
  return apiRequest(`/projects/${encode(projectID)}`, { method: 'PUT', body: JSON.stringify({ name }) })
}

export function deleteProject(projectID: string): Promise<void> {
  return apiRequest(`/projects/${encode(projectID)}`, { method: 'DELETE' })
}

export function restartProject(projectID: string): Promise<void> {
  return apiRequest(`/projects/${encode(projectID)}/restart`, { method: 'POST' })
}

export function listClusters(projectID: string): Promise<Cluster[]> {
  return apiRequest(`/projects/${encode(projectID)}/clusters`)
}

export function listProjectHosts(projectID: string): Promise<ProjectHost[]> {
  return apiRequest(`/projects/${encode(projectID)}/hosts`)
}

export function registerHost(hostID?: string): Promise<HostRegistration> {
  return apiRequest('/hosts', {
    method: 'POST',
    body: hostID ? JSON.stringify({ host_id: hostID }) : undefined,
  })
}

export function rotateHostToken(hostID: string): Promise<HostRegistration> {
  return apiRequest(`/hosts/${encode(hostID)}/token`, { method: 'POST' })
}

export function deleteUnusedHost(hostID: string): Promise<void> {
  return apiRequest(`/hosts/${encode(hostID)}`, { method: 'DELETE' })
}

export function createCluster(projectID: string, input: ClusterInput): Promise<Cluster> {
  return apiRequest(`/projects/${encode(projectID)}/clusters`, {
    method: 'POST',
    body: JSON.stringify(input),
  })
}

export function getCluster(clusterID: string): Promise<Cluster> {
  return apiRequest(`/clusters/${encode(clusterID)}`)
}

function putCluster(clusterID: string, input: ClusterInput): Promise<Cluster> {
  return apiRequest(`/clusters/${encode(clusterID)}`, {
    method: 'PUT',
    body: JSON.stringify(input),
  })
}

function clusterInput(cluster: Cluster, changes: Partial<ClusterInput>): ClusterInput {
  return {
    name: cluster.name,
    postgres_version: cluster.postgres_version,
    parameters: cluster.parameters,
    replica_count: cluster.replica_count,
    enabled_extensions: cluster.enabled_extensions,
    pg_hba_rules: cluster.pg_hba_rules,
    pgbouncer_enabled: cluster.pgbouncer_enabled,
    pg_bouncer: cluster.pg_bouncer,
    pg_back_rest: cluster.pg_back_rest,
    ...changes,
  }
}

type ClusterChanges = Partial<ClusterInput> | ((cluster: Cluster) => Partial<ClusterInput>)

export async function updateCluster(clusterID: string, changes: ClusterChanges): Promise<Cluster> {
  const current = await getCluster(clusterID)
  const resolvedChanges = typeof changes === 'function' ? changes(current) : changes
  return putCluster(current.id, clusterInput(current, resolvedChanges))
}

export function addReplica(cluster: Cluster): Promise<Cluster> {
  return updateCluster(cluster.id, (current) => ({ replica_count: current.replica_count + 1 }))
}

export function enablePgBouncer(cluster: Cluster, config: PgBouncerConfig): Promise<Cluster> {
  return updateCluster(cluster.id, { pgbouncer_enabled: true, pg_bouncer: config })
}

export function configurePgBackRest(cluster: Cluster, config: NonNullable<ClusterInput['pg_back_rest']>): Promise<Cluster> {
  return updateCluster(cluster.id, { pg_back_rest: config })
}

export function installExtension(cluster: Cluster, extension: string): Promise<Cluster> {
  return updateCluster(cluster.id, (current) => ({ enabled_extensions: [...new Set([...current.enabled_extensions, extension])].sort() }))
}

export function updatePgBouncer(clusterID: string, config: PgBouncerConfig): Promise<Cluster> {
  return apiRequest(`/clusters/${encode(clusterID)}/pgbouncer`, {
    method: 'PUT',
    body: JSON.stringify(config),
  })
}

export function updatePgHba(clusterID: string, rules: PgHbaRule[]): Promise<Cluster> {
  return apiRequest(`/clusters/${encode(clusterID)}/pg-hba`, {
    method: 'PUT',
    body: JSON.stringify({ rules }),
  })
}

export function updateParameters(clusterID: string, parameters: Record<string, string>): Promise<Cluster> {
  return apiRequest(`/clusters/${encode(clusterID)}/parameters`, {
    method: 'PUT',
    body: JSON.stringify({ parameters }),
  })
}

export function deleteCluster(clusterID: string): Promise<void> {
  return apiRequest(`/clusters/${encode(clusterID)}`, { method: 'DELETE' })
}

export async function getProjectTopology(projectID: string): Promise<ProjectTopology> {
  const [project, clusters] = await Promise.all([getProject(projectID), listClusters(projectID)])
  return { project, clusters }
}

export function listBackupJobs(projectID?: string): Promise<BackupJob[]> {
  const query = projectID ? `?project_id=${encode(projectID)}` : ''
  return apiRequest(`/backups${query}`)
}

export function createRestoreOperation(clusterID: string, input: CreateRestoreOperationInput, idempotencyKey: string): Promise<RestoreOperation> {
  return apiRequest(`/clusters/${encode(clusterID)}/restore-operations`, {
    method: 'POST',
    headers: { 'Idempotency-Key': idempotencyKey },
    body: JSON.stringify(input),
  })
}

export function listRestoreOperations(projectID: string): Promise<RestoreOperation[]> {
  return apiRequest(`/projects/${encode(projectID)}/restore-operations`)
}

export function getRestoreOperation(operationID: string): Promise<RestoreOperation> {
  return apiRequest(`/restore-operations/${encode(operationID)}`)
}

export function confirmRestoreOperation(operationID: string, confirmation: string): Promise<RestoreOperation> {
  return apiRequest(`/restore-operations/${encode(operationID)}/confirm`, {
    method: 'POST',
    body: JSON.stringify({ confirmation }),
  })
}

export type ConfirmedRestoreOperationAction = 'rollback' | 'finalize'

export function cancelRestoreOperation(operationID: string): Promise<RestoreOperation> {
  return apiRequest(`/restore-operations/${encode(operationID)}/cancel`, { method: 'POST' })
}

export function requestRestoreOperationAction(operationID: string, action: ConfirmedRestoreOperationAction, confirmation: string): Promise<RestoreOperation> {
  return apiRequest(`/restore-operations/${encode(operationID)}/${action}`, {
    method: 'POST',
    body: JSON.stringify({ confirmation }),
  })
}
