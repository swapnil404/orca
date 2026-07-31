import type { BackupJob, Cluster, ClusterInput, HostRegistration, PgBouncerConfig, PgHbaRule, Project, ProjectHost, ProjectTopology } from '../types/resources'
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

export function updateCluster(clusterID: string, input: ClusterInput): Promise<Cluster> {
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

export function addReplica(cluster: Cluster): Promise<Cluster> {
  return getCluster(cluster.id).then((current) => updateCluster(current.id, clusterInput(current, { replica_count: current.replica_count + 1 })))
}

export function enablePgBouncer(cluster: Cluster, config: PgBouncerConfig): Promise<Cluster> {
  return getCluster(cluster.id).then((current) => updateCluster(current.id, clusterInput(current, { pgbouncer_enabled: true, pg_bouncer: config })))
}

export function configurePgBackRest(cluster: Cluster, config: NonNullable<ClusterInput['pg_back_rest']>): Promise<Cluster> {
  return getCluster(cluster.id).then((current) => updateCluster(current.id, clusterInput(current, { pg_back_rest: config })))
}

export function installExtension(cluster: Cluster, extension: string): Promise<Cluster> {
  return getCluster(cluster.id).then((current) => {
    const enabledExtensions = [...new Set([...current.enabled_extensions, extension])].sort()
    return updateCluster(current.id, clusterInput(current, { enabled_extensions: enabledExtensions }))
  })
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
