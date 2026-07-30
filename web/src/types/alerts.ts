export type AlertComparison = 'gt' | 'gte' | 'lt' | 'lte' | 'eq' | 'neq'
export type AlertSeverity = 'info' | 'warning' | 'critical'
export type AlertRuleState = 'ok' | 'firing'

export interface AlertRule {
  id: string
  project_id: string
  cluster_id?: string
  metric_name: string
  comparison: AlertComparison
  threshold: number
  duration_before_firing_seconds: number
  current_state: AlertRuleState
  last_transition_at: string
  severity: AlertSeverity
}

export interface AlertIncident {
  id: number
  project_id: string
  rule_id: string
  metric_name: string
  comparison: AlertComparison
  threshold: number
  severity: AlertSeverity
  fired_at: string
  resolved_at: string | null
}

export interface ProjectAlerts {
  rules: AlertRule[]
  incidents: AlertIncident[]
}
