import { Link, createFileRoute, useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import type { FormEvent } from 'react'
import { ApiError, signup } from '../api'
import { OrcaLogo } from '../components/OrcaLogo'

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
      await navigate({ to: '/organizations' })
    } catch (cause) {
      setError(signupError(cause))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <main className="flex min-h-screen items-center justify-center bg-[var(--bg)] px-5 py-12 text-[var(--text)]">
      <section className="w-full max-w-[400px]">
       <div className="w-full max-w-[420px]">
        <div className="mb-6"><Link to="/organizations" className="inline-flex items-center gap-2.5"><OrcaLogo className="h-8 w-8" /><span className="text-lg font-semibold">Orca</span></Link></div>
        <div className="rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--card)] p-7 sm:p-8">
        <h1 className="text-xl font-semibold">Create account</h1>
        <p className="mb-6 mt-1 text-sm text-[var(--text-2)]">Create a control-plane account.</p>
        <form method="post" className="space-y-4" onSubmit={handleSubmit}>
          <label className="block text-sm font-medium text-[var(--text-2)]"><span className="mb-2 block">Email</span><input required name="email" type="email" autoComplete="email" className="w-full rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--panel)] px-3.5 py-3 text-sm text-[var(--text)] outline-none hover:border-[var(--text-3)] focus:border-[var(--accent)] focus:ring-2 focus:ring-[var(--accent-soft)]" /></label>
          <label className="block text-sm font-medium text-[var(--text-2)]"><span className="mb-2 block">Password</span><input required name="password" type="password" autoComplete="new-password" className="w-full rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--panel)] px-3.5 py-3 text-sm text-[var(--text)] outline-none hover:border-[var(--text-3)] focus:border-[var(--accent)] focus:ring-2 focus:ring-[var(--accent-soft)]" /></label>
          {error && <p role="alert" className="text-sm leading-5 text-[var(--critical)]">{error}</p>}
          <button type="submit" disabled={submitting} className="w-full rounded-[var(--radius-md)] bg-[var(--accent)] px-4 py-2.5 text-sm font-semibold text-[var(--accent-contrast)] hover:bg-[var(--accent-hover)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:ring-offset-2 focus:ring-offset-[var(--card)] disabled:cursor-not-allowed disabled:opacity-60">{submitting ? 'Creating account...' : 'Create account'}</button>
        </form>
        <p className="mt-7 text-center text-sm text-[var(--text-2)]">Already have an account? <Link to="/login" className="font-medium text-[var(--text)] underline decoration-[var(--text-3)] underline-offset-4 hover:decoration-[var(--accent)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]">Log in</Link></p>
        </div>
       </div>
      </section>
    </main>
  )
}
