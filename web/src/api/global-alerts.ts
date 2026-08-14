import { apiRequest } from './client'
import type { AlertComparison, AlertSeverity } from '../types/alerts'

export type { AlertComparison, AlertSeverity } from '../types/alerts'
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
