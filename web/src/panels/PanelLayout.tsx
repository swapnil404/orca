import type { ReactNode } from 'react'

interface PanelLayoutProps {
  title: string
  eyebrow: string
  onClose: () => void
  children: ReactNode
}

export function PanelLayout({ title, eyebrow, onClose, children }: PanelLayoutProps) {
  return (
    <section>
      <header className="mb-7 flex items-start justify-between gap-4 border-b border-[var(--border-soft)] pb-5">
        <div>
          <p className="mb-1 text-xs text-[var(--text-2)]">{eyebrow}</p>
          <h2 className="text-lg font-semibold text-[var(--text)]">{title}</h2>
        </div>
        <button type="button" onClick={onClose} aria-label="Close panel" className="grid h-8 w-8 place-items-center rounded-full border border-[var(--border)] text-lg text-[var(--text-2)] hover:border-[var(--text-3)] hover:bg-white/5 hover:text-[var(--text)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]">
          ×
        </button>
      </header>
      {children}
    </section>
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
