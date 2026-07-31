import type { Organization, OrganizationMember } from '../types/organizations'
import type { Project } from '../types/resources'
import { apiRequest } from './client'

const encode = encodeURIComponent

export function listOrganizations(): Promise<Organization[]> {
  return apiRequest('/orgs')
}

export function createOrganization(name: string): Promise<Organization> {
  return apiRequest('/orgs', { method: 'POST', body: JSON.stringify({ name }) })
}

export function updateOrganization(organizationID: string, name: string): Promise<Organization> {
  return apiRequest(`/orgs/${encode(organizationID)}`, { method: 'PUT', body: JSON.stringify({ name }) })
}

export function deleteOrganization(organizationID: string): Promise<void> {
  return apiRequest(`/orgs/${encode(organizationID)}`, { method: 'DELETE' })
}

export function listOrganizationMembers(organizationID: string): Promise<OrganizationMember[]> {
  return apiRequest(`/orgs/${encode(organizationID)}/members`)
}

export function listOrganizationProjects(organizationID: string): Promise<Project[]> {
  return apiRequest(`/orgs/${encode(organizationID)}/projects`)
}
