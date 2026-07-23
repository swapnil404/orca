import type { Cluster, ClusterInput, Project, ProjectTopology } from '../types/resources'
import { apiRequest } from './client'

const encode = encodeURIComponent

export function listProjects(): Promise<Project[]> {
  return apiRequest('/projects')
}

export function createProject(name: string): Promise<Project> {
  return apiRequest('/projects', { method: 'POST', body: JSON.stringify({ name }) })
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

export function listClusters(projectID: string): Promise<Cluster[]> {
  return apiRequest(`/projects/${encode(projectID)}/clusters`)
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

export function deleteCluster(clusterID: string): Promise<void> {
  return apiRequest(`/clusters/${encode(clusterID)}`, { method: 'DELETE' })
}

export async function getProjectTopology(projectID: string): Promise<ProjectTopology> {
  const [project, clusters] = await Promise.all([getProject(projectID), listClusters(projectID)])
  return { project, clusters }
}
