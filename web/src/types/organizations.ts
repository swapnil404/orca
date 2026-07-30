export interface Organization {
  id: string
  name: string
  slug: string
  created_at: string
}

export type OrganizationRole = 'owner' | 'admin' | 'member'

export interface OrganizationMember {
  id: string
  organization_id: string
  user_id: string
  role: OrganizationRole
  created_at: string
  email?: string
}
