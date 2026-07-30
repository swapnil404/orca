import { Link, createFileRoute, useNavigate } from '@tanstack/react-router'
import { Building2, Database, Info, Server, X } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { ApiError, createProject, listOrganizations } from '../../../api'

const projectNamePattern = /^[a-z0-9]+(?:-[a-z0-9]+)*$/

export const Route = createFileRoute('/_authenticated/projects/new')({
  ssr: false,
  loader: listOrganizations,
  component: NewProjectPage,
})

function NewProjectPage() {
  const organizations = Route.useLoaderData()
  const navigate = useNavigate()
  const [name, setName] = useState('')
  const [organizationID, setOrganizationID] = useState(organizations[0]?.id ?? '')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')
  const nameIsValid = projectNamePattern.test(name)
  const showNameError = name.length > 0 && !nameIsValid
  const canSubmit = nameIsValid && organizationID.length > 0 && !submitting

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!canSubmit) return

    setSubmitting(true)
    setError('')
    try {
      const project = await createProject(name, organizationID)
      await navigate({ to: '/projects/$projectId', params: { projectId: project.id } })
    } catch (cause) {
      const detail = cause instanceof ApiError ? cause.message : 'The server could not complete the request.'
      setError(`Could not create the project: ${detail}`)
      setSubmitting(false)
    }
  }

  return (
    <main className="min-h-[calc(100vh-56px)] px-4 py-8 text-[var(--text)] sm:px-6 lg:px-8 lg:py-12">
      <div className="mx-auto max-w-6xl">
        <header className="mb-8 flex items-start justify-between gap-4">
          <div>
            <p className="font-mono text-[10px] font-medium uppercase tracking-[0.16em] text-[var(--text-3)]">New project</p>
            <h1 className="mt-2 text-2xl font-semibold tracking-tight">Create a project</h1>
            <p className="mt-2 text-sm text-[var(--text-2)]">Create the control-plane project first, then connect the host that will run PostgreSQL.</p>
          </div>
          <Link to="/" aria-label="Cancel project creation" className="grid h-9 w-9 shrink-0 place-items-center rounded-full border border-[var(--border)] bg-[var(--panel)] text-[var(--text-2)] hover:border-[var(--text-3)] hover:text-[var(--text)]">
            <X aria-hidden="true" className="h-4 w-4" />
          </Link>
        </header>

        <div className="grid items-start gap-8 lg:grid-cols-[minmax(0,1fr)_360px]">
          <form onSubmit={handleSubmit} noValidate>
            {error && <div role="alert" className="mb-5 rounded-[var(--radius-sm)] border border-[color-mix(in_srgb,var(--critical)_45%,var(--border))] bg-[color-mix(in_srgb,var(--critical)_8%,var(--card))] px-4 py-3 text-sm text-[var(--critical)]">{error}</div>}

            <div className="space-y-7">
              <label className="block text-sm font-medium">
                Organization
                <select required value={organizationID} onChange={(event) => setOrganizationID(event.target.value)} className="mt-2 w-full rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--panel)] px-3 py-2.5 text-sm text-[var(--text)] outline-none focus:border-[var(--accent)] focus:ring-2 focus:ring-[var(--accent-soft)]">
                  {organizations.length === 0 && <option value="">No organizations available</option>}
                  {organizations.map((organization) => <option key={organization.id} value={organization.id}>{organization.name}</option>)}
                </select>
              </label>

              <label className="block text-sm font-medium">
                Project name
                <input autoFocus required value={name} onChange={(event) => setName(event.target.value)} aria-invalid={showNameError} aria-describedby="project-name-note" maxLength={100} placeholder="production-database" className={`mt-2 w-full rounded-[var(--radius-sm)] border bg-[var(--panel)] px-3 py-2.5 font-mono text-sm text-[var(--text)] outline-none placeholder:text-[var(--text-3)] focus:ring-2 focus:ring-[var(--accent-soft)] ${showNameError ? 'border-[var(--critical)]' : 'border-[var(--border)] focus:border-[var(--accent)]'}`} />
                <span id="project-name-note" className={`mt-2 block text-xs ${showNameError ? 'text-[var(--critical)]' : 'text-[var(--text-3)]'}`}>Lowercase alphanumeric characters and dashes only.</span>
              </label>

              <div>
                <p className="text-sm font-medium">PostgreSQL version and topology</p>
                <div className="mt-2 rounded-[var(--radius-lg)] border border-[var(--accent)] bg-[var(--accent-soft)] p-4">
                  <div className="flex gap-3">
                    <span className="grid h-9 w-9 shrink-0 place-items-center rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--card)] text-[var(--accent)]"><Database aria-hidden="true" className="h-4 w-4" /></span>
                    <div>
                      <p className="text-sm font-semibold">Configured after host connection</p>
                      <p className="mt-1 text-xs leading-5 text-[var(--text-2)]">Choose the PostgreSQL version and primary/replica topology when you provision a cluster on a registered host.</p>
                    </div>
                  </div>
                </div>
              </div>
            </div>

            <button type="submit" disabled={!canSubmit} className="mt-9 inline-flex min-w-40 items-center justify-center rounded-[var(--radius-sm)] bg-[var(--accent)] px-5 py-2.5 text-sm font-semibold text-[var(--accent-contrast)] transition duration-[var(--dur-fast)] ease-[var(--ease-premium)] hover:bg-[var(--accent-hover)] active:translate-y-px disabled:cursor-not-allowed disabled:opacity-45 disabled:hover:bg-[var(--accent)] disabled:active:translate-y-0">
              {submitting ? 'Creating...' : 'Create project'}
            </button>
          </form>

          <aside className="rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--card)] p-5 lg:sticky lg:top-8">
            <h2 className="text-sm font-semibold">Your project includes</h2>
            <div className="mt-4 divide-y divide-[var(--border-soft)] border-y border-[var(--border-soft)]">
              <div className="flex items-center gap-3 py-3.5">
                <Building2 aria-hidden="true" className="h-4 w-4 text-[var(--text-3)]" />
                <div><p className="text-sm font-medium">1 project shell</p><p className="mt-0.5 text-xs text-[var(--text-3)]">In {organizations.find((organization) => organization.id === organizationID)?.name ?? 'your organization'}</p></div>
              </div>
              <div className="flex items-center gap-3 py-3.5">
                <Server aria-hidden="true" className="h-4 w-4 text-[var(--text-3)]" />
                <div><p className="text-sm font-medium">No infrastructure yet</p><p className="mt-0.5 text-xs text-[var(--text-3)]">Nodes appear after a host is connected</p></div>
              </div>
            </div>
            <div className="mt-5 flex gap-3 rounded-[var(--radius-md)] bg-[var(--panel)] p-4 text-xs leading-5 text-[var(--text-2)]">
              <Info aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0 text-[var(--accent)]" />
              <p>Storage and compute are provided by hosts you register. You'll connect a host after creating this project.</p>
            </div>
          </aside>
        </div>
      </div>
    </main>
  )
}
