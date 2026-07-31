import { ArrowDown, ArrowUp, Plus, Trash2 } from 'lucide-react'
import type { PgHbaMethod, PgHbaRule, PgHbaType } from '../types/resources'

const fieldClass = 'w-full rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--panel)] px-2.5 py-2 font-mono text-xs text-[var(--text)] outline-none focus:border-[var(--accent)]'

export const defaultPgHbaRules: PgHbaRule[] = [
  { type: 'host', database: 'all', user: 'all', address: '0.0.0.0/0', method: 'reject' },
  { type: 'host', database: 'all', user: 'all', address: '::/0', method: 'reject' },
]

interface PgHbaRulesEditorProps {
  rules: PgHbaRule[]
  disabled?: boolean
  onChange: (rules: PgHbaRule[]) => void
}

export function PgHbaRulesEditor({ rules, disabled = false, onChange }: PgHbaRulesEditorProps) {
  function update(index: number, changes: Partial<PgHbaRule>) {
    onChange(rules.map((rule, ruleIndex) => ruleIndex === index ? { ...rule, ...changes } : rule))
  }

  function addRule() {
    onChange([...rules, { type: 'host', database: 'all', user: 'all', address: '10.0.0.0/8', method: 'scram-sha-256' }])
  }

  function moveRule(index: number, offset: -1 | 1) {
    const next = [...rules]
    const target = index + offset
    ;[next[index], next[target]] = [next[target], next[index]]
    onChange(next)
  }

  return <div className="space-y-3"><div className="hidden grid-cols-[110px_1fr_1fr_1.2fr_150px_112px] gap-2 px-1 font-mono text-[10px] uppercase tracking-wide text-[var(--text-3)] lg:grid"><span>Type</span><span>Database</span><span>User</span><span>Address / CIDR</span><span>Method</span><span>Order</span></div>{rules.map((rule, index) => <div key={index} className="grid gap-2 rounded-[var(--radius-md)] border border-[var(--border-soft)] bg-[var(--panel)] p-3 lg:grid-cols-[110px_1fr_1fr_1.2fr_150px_112px] lg:border-0 lg:bg-transparent lg:p-0"><select aria-label={`Rule ${index + 1} type`} value={rule.type} disabled={disabled} onChange={(event) => { const type = event.target.value as PgHbaType; update(index, { type, address: type === 'local' ? '' : rule.address || '10.0.0.0/8' }) }} className={fieldClass}><option value="host">host</option><option value="hostssl">hostssl</option><option value="local">local</option></select><input aria-label={`Rule ${index + 1} database`} required value={rule.database} disabled={disabled} onChange={(event) => update(index, { database: event.target.value })} className={fieldClass} placeholder="all" /><input aria-label={`Rule ${index + 1} user`} required value={rule.user} disabled={disabled} onChange={(event) => update(index, { user: event.target.value })} className={fieldClass} placeholder="all" /><input aria-label={`Rule ${index + 1} address`} required={rule.type !== 'local'} value={rule.address} disabled={disabled || rule.type === 'local'} onChange={(event) => update(index, { address: event.target.value })} className={fieldClass} placeholder={rule.type === 'local' ? 'Not used' : '10.0.0.0/8'} /><select aria-label={`Rule ${index + 1} method`} value={rule.method} disabled={disabled} onChange={(event) => update(index, { method: event.target.value as PgHbaMethod })} className={fieldClass}><option value="scram-sha-256">scram-sha-256</option><option value="md5">md5</option><option value="trust">trust</option><option value="reject">reject</option></select><div className="flex gap-1"><button type="button" aria-label={`Move rule ${index + 1} up`} disabled={disabled || index === 0} onClick={() => moveRule(index, -1)} className="grid h-9 w-9 place-items-center rounded-[var(--radius-sm)] border border-[var(--border)] text-[var(--text-3)] disabled:opacity-25"><ArrowUp className="h-3.5 w-3.5" /></button><button type="button" aria-label={`Move rule ${index + 1} down`} disabled={disabled || index === rules.length - 1} onClick={() => moveRule(index, 1)} className="grid h-9 w-9 place-items-center rounded-[var(--radius-sm)] border border-[var(--border)] text-[var(--text-3)] disabled:opacity-25"><ArrowDown className="h-3.5 w-3.5" /></button><button type="button" aria-label={`Remove rule ${index + 1}`} disabled={disabled} onClick={() => onChange(rules.filter((_, ruleIndex) => ruleIndex !== index))} className="grid h-9 w-9 place-items-center rounded-[var(--radius-sm)] border border-[var(--border)] text-[var(--text-3)] hover:border-[var(--critical)]/40 hover:text-[var(--critical)] disabled:opacity-40"><Trash2 className="h-3.5 w-3.5" /></button></div></div>)}<button type="button" disabled={disabled} onClick={addRule} className="inline-flex items-center gap-2 rounded-[var(--radius-sm)] border border-dashed border-[var(--border)] px-3 py-2 text-xs font-medium text-[var(--text-2)] hover:border-[var(--accent)] hover:text-[var(--accent)] disabled:opacity-40"><Plus className="h-3.5 w-3.5" />Add rule</button></div>
}

export function pgHbaRulesValid(rules: PgHbaRule[]): boolean {
  return rules.every((rule) => rule.database.length > 0 && rule.user.length > 0 && (rule.type === 'local' ? rule.address === '' : rule.address.length > 0))
}
