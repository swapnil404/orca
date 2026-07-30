import type { ProjectAlerts } from '../types/alerts'
import { apiRequest } from './client'

export function getProjectAlerts(projectID: string): Promise<ProjectAlerts> {
  return apiRequest(`/projects/${encodeURIComponent(projectID)}/alerts`)
}
