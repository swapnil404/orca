import type { ReactNode } from 'react'
import { motion, useReducedMotion } from 'framer-motion'

interface PanelLayoutProps {
  title: string
  eyebrow: string
  onClose: () => void
  children: ReactNode
}

export function PanelLayout({ title, eyebrow, onClose, children }: PanelLayoutProps) {
  const reduceMotion = useReducedMotion()

  return (
    <motion.aside initial={{ opacity: 0, x: reduceMotion ? 0 : 32 }} animate={{ opacity: 1, x: 0 }} transition={{ duration: reduceMotion ? 0.01 : 0.36, ease: [0.22, 1, 0.36, 1] }} className="absolute inset-y-3 right-3 z-10 w-[min(390px,calc(100%-24px))] overflow-y-auto rounded-[var(--radius-xl)] border border-[var(--border)] bg-[var(--panel)] p-5 shadow-[0_30px_90px_rgba(0,0,0,0.42)] sm:inset-y-4 sm:right-4 sm:p-6">
      <header className="mb-7 flex items-start justify-between gap-4 border-b border-[var(--border-soft)] pb-5">
        <div>
          <p className="mb-2 font-mono text-[9px] font-semibold uppercase tracking-[0.2em] text-[var(--accent)]">{eyebrow}</p>
          <h2 className="text-xl font-semibold tracking-[-0.015em] text-[var(--text)]">{title}</h2>
        </div>
        <button type="button" onClick={onClose} aria-label="Close panel" className="grid h-8 w-8 place-items-center rounded-full border border-[var(--border)] text-lg text-[var(--text-2)] transition hover:border-[var(--text-3)] hover:bg-white/5 hover:text-[var(--text)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]">
          ×
        </button>
      </header>
      {children}
    </motion.aside>
  )
}

interface StateRowProps {
  label: string
  value: ReactNode
}

export function StateRow({ label, value }: StateRowProps) {
  return (
    <div className="flex items-start justify-between gap-6 border-b border-[var(--border-soft)] py-3.5 text-sm last:border-0">
      <dt className="text-[var(--text-3)]">{label}</dt>
      <dd className="max-w-[60%] break-words text-right font-mono text-xs text-[var(--text)]">{value}</dd>
    </div>
  )
}

export function ContractUnavailable() {
  return <p className="rounded-[var(--radius-md)] border border-amber-300/15 bg-amber-300/[0.04] p-4 text-sm leading-6 text-amber-100/80">Not available in the current server desired-state or actual-state contract.</p>
}
