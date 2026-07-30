interface PlaceholderFeature {
  title: string
  description: string
}

interface PlaceholderPageProps {
  eyebrow: string
  title: string
  description: string
  features: PlaceholderFeature[]
}

export function PlaceholderPage({ eyebrow, title, description, features }: PlaceholderPageProps) {
  return (
    <main className="min-h-[calc(100vh-56px)] px-4 py-6 text-[var(--text)] sm:px-6 lg:px-8">
      <div className="mx-auto max-w-7xl">
        <header>
          <h1 className="text-xl font-semibold">{title}</h1>
          <p className="mt-1 max-w-3xl text-sm text-[var(--text-2)]">{description}</p>
        </header>

        <section className="mt-5 overflow-hidden rounded-[var(--radius-sm)] border border-[var(--border)]">
          <div className="flex items-center justify-between border-b border-[var(--border)] bg-[var(--panel)] px-4 py-3">
            <h2 className="text-sm font-medium">{eyebrow}</h2>
            <span className="text-xs text-[var(--warning)]">Not implemented</span>
          </div>
          <table className="w-full border-collapse text-left text-sm">
            <thead className="text-xs text-[var(--text-2)]"><tr><th className="px-4 py-3 font-medium">Capability</th><th className="hidden px-4 py-3 font-medium sm:table-cell">Description</th><th className="px-4 py-3 text-right font-medium">Status</th></tr></thead>
            <tbody className="divide-y divide-[var(--border-soft)] bg-[var(--card)]">
              {features.map((feature) => <tr key={feature.title}><td className="px-4 py-3 font-medium">{feature.title}<p className="mt-1 font-normal text-[var(--text-2)] sm:hidden">{feature.description}</p></td><td className="hidden px-4 py-3 text-[var(--text-2)] sm:table-cell">{feature.description}</td><td className="px-4 py-3 text-right text-xs text-[var(--text-3)]">Unavailable</td></tr>)}
            </tbody>
          </table>
        </section>
      </div>
    </main>
  )
}
