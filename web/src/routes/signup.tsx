import { Link, createFileRoute, useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import type { FormEvent } from 'react'
import { ApiError, signup } from '../api'
import { OrcaLogo } from '../components/OrcaLogo'
import { BrushHighlight } from '../components/BrushHighlight'

export const Route = createFileRoute('/signup')({
  component: SignupPage,
})

function signupError(error: unknown): string {
  if (error instanceof ApiError && error.status === 409) {
    return 'An account with this email already exists'
  }
  if (error instanceof ApiError && error.status === 0) {
    return 'Authentication service is unavailable'
  }
  return error instanceof ApiError ? error.message : 'Account creation failed'
}

function SignupPage() {
  const navigate = useNavigate()
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = new FormData(event.currentTarget)
    setSubmitting(true)
    setError('')
    try {
      await signup(String(form.get('email') ?? ''), String(form.get('password') ?? ''))
      await navigate({ to: '/' })
    } catch (cause) {
      setError(signupError(cause))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <main className="relative grid min-h-screen overflow-hidden bg-[var(--bg)] text-[var(--text)] lg:grid-cols-[minmax(360px,0.9fr)_1.1fr]">
      <section className="relative hidden overflow-hidden border-r border-[var(--border-soft)] p-10 lg:flex lg:flex-col lg:justify-between xl:p-14">
        <Link to="/" aria-label="Orca home" className="inline-flex w-fit items-center gap-2.5 rounded-lg focus:outline-none focus:ring-2 focus:ring-[var(--accent)]"><OrcaLogo className="h-8 w-8" /><span className="text-lg font-semibold tracking-[-0.02em]">orca</span></Link>
        <div className="max-w-lg">
          <p className="font-mono text-[10px] font-semibold uppercase tracking-[0.24em] text-[var(--accent)]">Built for operators</p>
          <h2 className="mt-5 text-4xl font-semibold leading-[1.12] tracking-[-0.025em] xl:text-5xl">Control without<br /><BrushHighlight>compromise.</BrushHighlight></h2>
          <p className="mt-5 max-w-md text-sm leading-6 text-[var(--text-2)]">Your Postgres data stays on your infrastructure. Orca stores desired state and health, never your application data.</p>
          <div className="mt-10 rounded-[var(--radius-lg)] border border-[var(--border)] bg-black/15 p-5">
            <div className="flex items-center gap-2 font-mono text-[9px] uppercase tracking-[0.16em] text-[var(--healthy)]"><span className="h-1.5 w-1.5 rounded-full bg-[var(--healthy)]" /> Outbound connection only</div>
            <p className="mt-3 text-xs leading-5 text-[var(--text-3)]">No inbound port required on your infrastructure.</p>
          </div>
        </div>
        <p className="font-mono text-[9px] uppercase tracking-[0.18em] text-[var(--text-3)]">Secure control plane / v1</p>
      </section>
      <section className="relative flex min-h-screen items-center justify-center px-5 py-12 sm:px-10">
       <div className="w-full max-w-[420px]">
        <div className="mb-12 lg:hidden"><Link to="/" className="inline-flex items-center gap-2.5"><OrcaLogo className="h-8 w-8" /><span className="text-lg font-semibold">orca</span></Link></div>
        <div className="rounded-[var(--radius-xl)] border border-[var(--border)] bg-[var(--card)] p-7 shadow-[0_32px_90px_rgba(0,0,0,0.4),inset_0_1px_rgba(255,255,255,0.035)] sm:p-9">
        <p className="font-mono text-[10px] font-semibold uppercase tracking-[0.2em] text-[var(--accent)]">Create your account</p>
        <h1 className="mb-2 mt-3 text-3xl font-semibold tracking-[-0.025em]">Start with Orca</h1>
        <p className="mb-8 text-sm text-[var(--text-2)]">Create your control plane workspace in seconds.</p>
        <form method="post" className="space-y-4" onSubmit={handleSubmit}>
          <label className="block text-sm font-medium text-[var(--text-2)]"><span className="mb-2 block">Email</span><input required name="email" type="email" autoComplete="email" className="w-full rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--panel)] px-3.5 py-3 text-sm text-[var(--text)] outline-none transition duration-[var(--dur-fast)] hover:border-[var(--text-3)] focus:border-[var(--accent)] focus:ring-2 focus:ring-[var(--accent-soft)]" /></label>
          <label className="block text-sm font-medium text-[var(--text-2)]"><span className="mb-2 block">Password</span><input required name="password" type="password" autoComplete="new-password" className="w-full rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--panel)] px-3.5 py-3 text-sm text-[var(--text)] outline-none transition duration-[var(--dur-fast)] hover:border-[var(--text-3)] focus:border-[var(--accent)] focus:ring-2 focus:ring-[var(--accent-soft)]" /></label>
          {error && <p role="alert" className="text-sm leading-5 text-[var(--critical)]">{error}</p>}
          <button type="submit" disabled={submitting} className="w-full rounded-[var(--radius-md)] bg-[var(--accent)] px-4 py-2.5 text-sm font-semibold text-[var(--accent-contrast)] transition duration-[var(--dur-fast)] hover:bg-[var(--accent-hover)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:ring-offset-2 focus:ring-offset-[var(--card)] disabled:cursor-not-allowed disabled:opacity-60">{submitting ? 'Creating account...' : 'Create account'}</button>
        </form>
        <p className="mt-7 text-center text-sm text-[var(--text-2)]">Already have an account? <Link to="/login" className="font-medium text-[var(--text)] underline decoration-[var(--text-3)] underline-offset-4 hover:decoration-[var(--accent)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]">Log in</Link></p>
        </div>
        <p className="mt-6 text-center text-[11px] text-[var(--text-3)]">By continuing, you agree to secure and responsible use.</p>
       </div>
      </section>
    </main>
  )
}
