import type { ReactNode } from 'react'

interface PanelLayoutProps {
  title: string
  eyebrow: string
  onClose: () => void
  children: ReactNode
}

export function PanelLayout({ title, eyebrow, onClose, children }: PanelLayoutProps) {
  return (
    <aside className="absolute inset-y-3 right-3 z-10 w-[min(390px,calc(100%-24px))] overflow-y-auto rounded-2xl border border-white/10 bg-[#0d1a17]/98 p-5 shadow-2xl shadow-black/50">
      <header className="mb-6 flex items-start justify-between gap-4">
        <div>
          <p className="mb-1 text-[10px] font-bold uppercase tracking-[0.2em] text-emerald-300">{eyebrow}</p>
          <h2 className="text-xl font-semibold text-white">{title}</h2>
        </div>
        <button type="button" onClick={onClose} className="rounded-lg border border-white/10 px-2.5 py-1.5 text-xs text-slate-300 hover:bg-white/5">
          Close
        </button>
      </header>
      {children}
    </aside>
  )
}

interface StateRowProps {
  label: string
  value: ReactNode
}

export function StateRow({ label, value }: StateRowProps) {
  return (
    <div className="flex items-start justify-between gap-6 border-b border-white/7 py-3 text-sm">
      <dt className="text-slate-500">{label}</dt>
      <dd className="max-w-[60%] break-words text-right text-slate-200">{value}</dd>
    </div>
  )
}

export function ContractUnavailable() {
  return <p className="rounded-xl border border-amber-300/20 bg-amber-300/5 p-4 text-sm leading-6 text-amber-100">Not available in the current server desired-state or actual-state contract.</p>
}
