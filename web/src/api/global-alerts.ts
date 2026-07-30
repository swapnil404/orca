import { apiRequest } from './client'

export type AlertComparison = 'gt' | 'gte' | 'lt' | 'lte' | 'eq' | 'neq'
export type AlertSeverity = 'info' | 'warning' | 'critical'
export type AlertStatus = 'firing' | 'resolved'

export interface GlobalAlertIncident {
  id: number
  project_id: string
  project_name: string
  rule_id: string
  metric_name: string
  comparison: AlertComparison
  threshold: number
  severity: AlertSeverity
  fired_at: string
  resolved_at: string | null
}

export function listGlobalAlerts(): Promise<GlobalAlertIncident[]> {
  return apiRequest('/alerts')
}
