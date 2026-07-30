import type { Organization, OrganizationMember } from '../types/organizations'
import { apiRequest } from './client'

const encode = encodeURIComponent

export function listOrganizations(): Promise<Organization[]> {
  return apiRequest('/orgs')
}

export function listOrganizationMembers(organizationID: string): Promise<OrganizationMember[]> {
  return apiRequest(`/orgs/${encode(organizationID)}/members`)
}
