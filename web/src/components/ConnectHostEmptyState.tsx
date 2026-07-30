import { RadioTower } from 'lucide-react'

interface ConnectHostEmptyStateProps {
  title: string
  description: string
  className?: string
}

export function ConnectHostEmptyState({ title, description, className = '' }: ConnectHostEmptyStateProps) {
  return (
    <section className={`grid min-h-72 place-items-center rounded-[var(--radius-lg)] border border-dashed border-[var(--border)] bg-[var(--card)] p-8 text-center ${className}`}>
      <div className="max-w-md">
        <span className="mx-auto grid h-11 w-11 place-items-center rounded-full border border-[var(--border)] bg-[var(--panel)] text-[var(--accent)]">
          <RadioTower aria-hidden="true" className="h-5 w-5" />
        </span>
        <h2 className="mt-4 text-base font-semibold">{title}</h2>
        <p className="mt-2 text-sm leading-6 text-[var(--text-2)]">{description}</p>
      </div>
    </section>
  )
}
