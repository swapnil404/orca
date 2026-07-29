import { Link, createFileRoute, useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import type { FormEvent } from 'react'
import { ApiError, signup } from '../api'

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
    <main className="relative grid min-h-screen place-items-center overflow-hidden bg-[var(--bg)] px-5 py-12 text-[var(--text)]">
      <div aria-hidden="true" className="absolute left-1/2 top-0 h-px w-[min(720px,90vw)] -translate-x-1/2 bg-gradient-to-r from-transparent via-[var(--accent)] to-transparent opacity-50" />
      <section className="relative w-full max-w-[380px] rounded-[var(--radius-xl)] border border-[var(--border)] bg-[var(--card)] p-7 shadow-[0_28px_80px_rgba(0,0,0,0.45)] sm:p-8">
        <p className="font-mono text-[11px] font-medium uppercase tracking-[0.2em] text-[var(--accent)]">Create your account</p>
        <h1 className="mb-7 mt-2 text-2xl font-semibold tracking-[-0.03em]">Start with Orca</h1>
        <form method="post" className="space-y-4" onSubmit={handleSubmit}>
          <label className="block text-sm font-medium text-[var(--text-2)]"><span className="mb-2 block">Email</span><input required name="email" type="email" autoComplete="email" className="w-full rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--panel)] px-3.5 py-2.5 text-[var(--text)] outline-none transition duration-[var(--dur-fast)] focus:border-transparent focus:ring-2 focus:ring-[var(--accent)]" /></label>
          <label className="block text-sm font-medium text-[var(--text-2)]"><span className="mb-2 block">Password</span><input required name="password" type="password" autoComplete="new-password" className="w-full rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--panel)] px-3.5 py-2.5 text-[var(--text)] outline-none transition duration-[var(--dur-fast)] focus:border-transparent focus:ring-2 focus:ring-[var(--accent)]" /></label>
          {error && <p role="alert" className="text-sm leading-5 text-[var(--critical)]">{error}</p>}
          <button type="submit" disabled={submitting} className="w-full rounded-[var(--radius-md)] bg-[var(--accent)] px-4 py-2.5 text-sm font-semibold text-[#111205] transition duration-[var(--dur-fast)] hover:bg-[#fbff91] focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:ring-offset-2 focus:ring-offset-[var(--card)] disabled:cursor-not-allowed disabled:opacity-60">{submitting ? 'Creating account...' : 'Create account'}</button>
        </form>
        <p className="mt-7 text-center text-sm text-[var(--text-2)]">Already have an account? <Link to="/login" className="font-medium text-[var(--text)] underline decoration-[var(--text-3)] underline-offset-4 hover:decoration-[var(--accent)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]">Log in</Link></p>
      </section>
    </main>
  )
}
