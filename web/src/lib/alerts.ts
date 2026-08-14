import type { AlertComparison } from '../types/alerts'

const comparisonLabels: Record<AlertComparison, string> = {
  gt: '>',
  gte: '>=',
  lt: '<',
  lte: '<=',
  eq: '=',
  neq: '!=',
}

export function alertComparisonLabel(comparison: AlertComparison): string {
  return comparisonLabels[comparison]
}

export function alertRuleLabel(rule: { metric_name: string; comparison: AlertComparison; threshold: number }): string {
  return `${rule.metric_name} ${alertComparisonLabel(rule.comparison)} ${rule.threshold}`
}
