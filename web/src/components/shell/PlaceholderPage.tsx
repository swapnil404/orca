import { motion, useReducedMotion } from 'framer-motion'
import { BrushHighlight } from '../BrushHighlight'

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
  const reduceMotion = useReducedMotion()

  return (
    <main className="min-h-[calc(100vh-64px)] px-5 py-10 text-[var(--text)] sm:min-h-screen sm:px-10 sm:py-14 lg:px-14">
      <div className="mx-auto max-w-5xl">
        <header className="max-w-2xl">
          <div className="mb-5 inline-flex items-center gap-2 rounded-full border border-[var(--border)] bg-[var(--panel)] px-3 py-1.5 font-mono text-[9px] font-semibold uppercase tracking-[0.18em] text-[var(--accent)]">
            <span className="h-1.5 w-1.5 rounded-full bg-[var(--accent)]" />
            {eyebrow}
          </div>
          <h1 className="text-4xl font-semibold tracking-[-0.025em] sm:text-5xl"><BrushHighlight>{title}</BrushHighlight></h1>
          <p className="mt-4 text-sm leading-7 text-[var(--text-2)] sm:text-base">{description}</p>
        </header>

        <section aria-label={`${title} capabilities`} className="mt-12 grid gap-4 md:grid-cols-3">
          {features.map((feature, index) => (
            <motion.article
              key={feature.title}
              initial={{ opacity: 0, y: reduceMotion ? 0 : 10 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ duration: reduceMotion ? 0.01 : 0.42, delay: reduceMotion ? 0 : index * 0.07, ease: [0.22, 1, 0.36, 1] }}
              className="rounded-[var(--radius-xl)] border border-[var(--border)] bg-[var(--card)] p-6"
            >
              <span className="inline-grid h-8 w-8 place-items-center rounded-full bg-[var(--accent)] font-mono text-[10px] font-semibold text-[var(--accent-contrast)]">0{index + 1}</span>
              <h2 className="mt-8 text-base font-semibold tracking-[-0.02em]">{feature.title}</h2>
              <p className="mt-2 text-sm leading-6 text-[var(--text-2)]">{feature.description}</p>
            </motion.article>
          ))}
        </section>

        <div className="mt-8 flex flex-col gap-3 rounded-[var(--radius-lg)] border border-dashed border-[var(--border)] bg-[var(--panel)] p-5 sm:flex-row sm:items-center sm:justify-between">
          <div><p className="text-sm font-medium">This workspace is being prepared</p><p className="mt-1 text-xs text-[var(--text-3)]">The navigation is ready; functionality will arrive in a future release.</p></div>
          <span className="w-fit rounded-full border border-[var(--border)] bg-[var(--card)] px-3 py-1.5 font-mono text-[9px] uppercase tracking-[0.14em] text-[var(--text-2)]">Coming soon</span>
        </div>
      </div>
    </main>
  )
}
