import { Link, createFileRoute, useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import type { FormEvent } from 'react'
import { ApiError, login, startOAuth } from '../api'
import { OrcaLogo } from '../components/OrcaLogo'

interface LoginSearch {
  oauth_error?: string
}

export const Route = createFileRoute('/login')({
  validateSearch: (search: Record<string, unknown>): LoginSearch => ({
    oauth_error: typeof search.oauth_error === 'string' ? search.oauth_error : undefined,
  }),
  component: LoginPage,
})

function loginError(error: unknown): string {
  if (error instanceof ApiError && error.status === 401) {
    return 'Incorrect email or password'
  }
  if (error instanceof ApiError && error.status === 0) {
    return 'Authentication service is unavailable'
  }
  return error instanceof ApiError ? error.message : 'Login failed'
}

function LoginPage() {
  const navigate = useNavigate()
  const search = Route.useSearch()
  const [error, setError] = useState(search.oauth_error === 'provider_unavailable' ? 'This OAuth provider is not configured' : search.oauth_error ? 'OAuth authentication failed' : '')
  const [submitting, setSubmitting] = useState(false)

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = new FormData(event.currentTarget)
    setSubmitting(true)
    setError('')
    try {
      await login(String(form.get('email') ?? ''), String(form.get('password') ?? ''))
      await navigate({ to: '/organizations' })
    } catch (cause) {
      setError(loginError(cause))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <AuthPage eyebrow="Control plane access" title="Log in to Orca">
      <form method="post" className="space-y-4" onSubmit={handleSubmit}>
        <AuthField label="Email" name="email" type="email" autoComplete="email" />
        <AuthField label="Password" name="password" type="password" autoComplete="current-password" />
        {error && <p role="alert" className="text-sm leading-5 text-[var(--critical)]">{error}</p>}
        <button type="submit" disabled={submitting} className="w-full rounded-[var(--radius-md)] bg-[var(--accent)] px-4 py-2.5 text-sm font-semibold text-[var(--accent-contrast)] hover:bg-[var(--accent-hover)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:ring-offset-2 focus:ring-offset-[var(--card)] disabled:cursor-not-allowed disabled:opacity-60">
          {submitting ? 'Logging in...' : 'Log in'}
        </button>
      </form>
      <div className="my-6 flex items-center gap-3 text-xs text-[var(--text-3)]"><span className="h-px flex-1 bg-[var(--border-soft)]" /><span>or</span><span className="h-px flex-1 bg-[var(--border-soft)]" /></div>
      <div className="grid gap-3">
        <OAuthButton provider="github" label="Continue with GitHub"><GitHubIcon /></OAuthButton>
        <OAuthButton provider="google" label="Continue with Google"><GoogleIcon /></OAuthButton>
      </div>
      <p className="mt-7 text-center text-sm text-[var(--text-2)]">New to Orca? <Link to="/signup" className="font-medium text-[var(--text)] underline decoration-[var(--text-3)] underline-offset-4 hover:decoration-[var(--accent)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]">Create an account</Link></p>
    </AuthPage>
  )
}

interface AuthPageProps {
  eyebrow: string
  title: string
  children: React.ReactNode
}

function AuthPage({ eyebrow, title, children }: AuthPageProps) {
  return (
    <main className="flex min-h-screen items-center justify-center bg-[var(--bg)] px-5 py-12 text-[var(--text)]">
      <section className="w-full max-w-[400px]">
        <div className="w-full max-w-[420px]">
          <div className="mb-6"><AuthBrand /></div>
          <div className="rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--card)] p-7 sm:p-8">
            <h1 className="text-xl font-semibold">{title}</h1>
            <p className="mb-6 mt-1 text-sm text-[var(--text-2)]">{eyebrow}</p>
            {children}
          </div>
        </div>
      </section>
    </main>
  )
}

function AuthBrand() {
  return <Link to="/organizations" aria-label="Orca home" className="inline-flex w-fit items-center gap-2.5 rounded-lg focus:outline-none focus:ring-2 focus:ring-[var(--accent)]"><OrcaLogo className="h-8 w-8" /><span className="text-lg font-semibold tracking-[-0.02em]">orca</span></Link>
}

interface AuthFieldProps {
  label: string
  name: string
  type: 'email' | 'password' | 'text'
  autoComplete: string
}

function AuthField({ label, name, type, autoComplete }: AuthFieldProps) {
  return (
    <label className="block text-sm font-medium text-[var(--text-2)]">
      <span className="mb-2 block">{label}</span>
      <input required name={name} type={type} autoComplete={autoComplete} className="w-full rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--panel)] px-3.5 py-3 text-sm text-[var(--text)] outline-none placeholder:text-[var(--text-3)] hover:border-[var(--text-3)] focus:border-[var(--accent)] focus:ring-2 focus:ring-[var(--accent-soft)]" />
    </label>
  )
}

interface OAuthButtonProps {
  provider: 'github' | 'google'
  label: string
  children: React.ReactNode
}

function OAuthButton({ provider, label, children }: OAuthButtonProps) {
  return <button type="button" onClick={() => startOAuth(provider)} className="flex w-full items-center justify-center gap-2.5 rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--panel)] px-4 py-2.5 text-sm font-medium text-[var(--text)] hover:border-[var(--accent)] hover:bg-[var(--card-raised)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]">{children}{label}</button>
}

function GitHubIcon() {
  return <svg aria-hidden="true" viewBox="0 0 24 24" className="h-4 w-4 fill-current"><path d="M12 2a10 10 0 0 0-3.16 19.49c.5.09.68-.22.68-.48v-1.87c-2.78.6-3.37-1.18-3.37-1.18-.45-1.16-1.11-1.47-1.11-1.47-.91-.62.07-.61.07-.61 1 .07 1.53 1.03 1.53 1.03.9 1.53 2.35 1.09 2.92.83.09-.65.35-1.09.64-1.34-2.22-.25-4.55-1.11-4.55-4.94 0-1.09.39-1.98 1.03-2.68-.1-.25-.45-1.27.1-2.64 0 0 .84-.27 2.75 1.02A9.6 9.6 0 0 1 12 6.82a9.6 9.6 0 0 1 2.5.34c1.91-1.29 2.75-1.02 2.75-1.02.55 1.37.2 2.39.1 2.64.64.7 1.03 1.59 1.03 2.68 0 3.84-2.34 4.68-4.57 4.93.36.31.68.92.68 1.86v2.76c0 .27.18.58.69.48A10 10 0 0 0 12 2Z" /></svg>
}

function GoogleIcon() {
  return <svg aria-hidden="true" viewBox="0 0 24 24" className="h-4 w-4"><path fill="#4285F4" d="M21.6 12.23c0-.71-.06-1.4-.18-2.07H12v3.92h5.38a4.6 4.6 0 0 1-2 3.02v2.54h3.24c1.9-1.75 2.98-4.32 2.98-7.41Z" /><path fill="#34A853" d="M12 22c2.7 0 4.98-.9 6.63-2.36l-3.25-2.54c-.9.6-2.05.96-3.38.96-2.61 0-4.82-1.76-5.61-4.13H3.03v2.62A10 10 0 0 0 12 22Z" /><path fill="#FBBC05" d="M6.39 13.93A6 6 0 0 1 6.08 12c0-.67.12-1.32.31-1.93V7.45H3.03A10 10 0 0 0 2 12c0 1.64.39 3.19 1.03 4.55l3.36-2.62Z" /><path fill="#EA4335" d="M12 5.94c1.47 0 2.79.5 3.83 1.5l2.87-2.88A9.62 9.62 0 0 0 12 2a10 10 0 0 0-8.97 5.45l3.36 2.62C7.18 7.7 9.39 5.94 12 5.94Z" /></svg>
}
