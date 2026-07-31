import { Link, createFileRoute, useNavigate, useRouter } from '@tanstack/react-router'
import { ArrowUpRight, Building2, FolderKanban, Plus, Users, X } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { ApiError, createOrganization, listOrganizationMembers, listOrganizationProjects, listOrganizations } from '../../api'
import type { Organization, OrganizationMember } from '../../types/organizations'
import type { Project } from '../../types/resources'

interface OrganizationSummary {
  organization: Organization
  members: OrganizationMember[]
  projects: Project[]
}

async function loadOrganizations(): Promise<OrganizationSummary[]> {
  const organizations = await listOrganizations()
  return Promise.all(organizations.map(async (organization) => ({
    organization,
    members: await listOrganizationMembers(organization.id),
    projects: await listOrganizationProjects(organization.id),
  })))
}

export const Route = createFileRoute('/_authenticated/organizations')({
  ssr: false,
  loader: loadOrganizations,
  component: OrganizationsPage,
})

function OrganizationsPage() {
  const organizations = Route.useLoaderData()
  const navigate = useNavigate()
  const router = useRouter()
  const [creating, setCreating] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')
  const memberCount = organizations.reduce((total, item) => total + item.members.length, 0)
  const projectCount = organizations.reduce((total, item) => total + item.projects.length, 0)

  async function handleCreate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const name = String(new FormData(event.currentTarget).get('name') ?? '').trim()
    if (!name) return
    setSubmitting(true)
    setError('')
    try {
      const organization = await createOrganization(name)
      await router.invalidate()
      await navigate({ to: '/settings', search: { organizationId: organization.id } })
    } catch (cause) {
      setError(cause instanceof ApiError ? cause.message : 'Could not create the organization.')
      setSubmitting(false)
    }
  }

  return (
    <main className="relative min-h-[calc(100vh-64px)] overflow-hidden px-4 py-8 text-[var(--text)] sm:px-6 lg:px-10 lg:py-12">
      <div aria-hidden="true" className="pointer-events-none absolute -left-64 top-20 h-[520px] w-[520px] rounded-full bg-[var(--streaming)] opacity-[0.025] blur-3xl" />
      <div className="relative mx-auto max-w-7xl">
        <header className="grid gap-8 border-b border-[var(--border)] pb-8 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-end">
          <div><h1 className="text-3xl font-semibold tracking-[-0.045em] sm:text-4xl">Your <span className="text-[var(--accent)]">organizations</span></h1><p className="mt-3 max-w-xl text-sm leading-6 text-[var(--text-2)]">Isolate projects, infrastructure, and team access into deliberate operational workspaces.</p></div>
          <button type="button" onClick={() => { setCreating((current) => !current); setError('') }} className={`inline-flex h-10 items-center justify-center gap-2 rounded-[var(--radius-sm)] px-4 text-xs font-semibold transition-colors ${creating ? 'border border-[var(--border)] bg-[var(--card)] text-[var(--text-2)] hover:text-[var(--text)]' : 'bg-[var(--accent)] text-[var(--accent-contrast)] hover:bg-[var(--accent-hover)]'}`}>{creating ? <X className="h-3.5 w-3.5" /> : <Plus className="h-3.5 w-3.5" />}{creating ? 'Close composer' : 'New organization'}</button>
        </header>

        <div className="grid grid-cols-3 gap-px overflow-hidden rounded-b-[var(--radius-md)] border-x border-b border-[var(--border)] bg-[var(--border)] sm:w-fit">
          <OrgMetric value={organizations.length} label="Organizations" />
          <OrgMetric value={projectCount} label="Projects" />
          <OrgMetric value={memberCount} label="Memberships" />
        </div>

        {creating && <form onSubmit={handleCreate} className="mt-8 overflow-hidden rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--card)]"><div className="grid md:grid-cols-[220px_minmax(0,1fr)]"><div className="border-b border-[var(--border)] bg-[var(--panel)] p-5 md:border-b-0 md:border-r"><span className="grid h-9 w-9 place-items-center rounded-[var(--radius-sm)] bg-[var(--accent-soft)] text-[var(--accent)]"><Building2 className="h-4 w-4" /></span><h2 className="mt-4 text-sm font-semibold">Create a workspace</h2><p className="mt-1 text-xs leading-5 text-[var(--text-3)]">Projects and members will live inside this boundary.</p></div><div className="p-5"><label className="block text-[10px] font-medium uppercase tracking-[0.12em] text-[var(--text-3)]">Organization name<input name="name" required autoFocus placeholder="Acme infrastructure" className="mt-2 h-11 w-full rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--bg)] px-3 text-sm normal-case tracking-normal text-[var(--text)] outline-none transition-shadow placeholder:text-[var(--text-3)] focus:border-[var(--accent)] focus:ring-2 focus:ring-[var(--accent-soft)]" /></label><div className="mt-4 flex items-center justify-between gap-4"><p className="text-xs text-[var(--text-3)]">A unique URL slug will be generated automatically.</p><button disabled={submitting} className="shrink-0 rounded-[var(--radius-sm)] bg-[var(--accent)] px-4 py-2.5 text-xs font-semibold text-[var(--accent-contrast)] hover:bg-[var(--accent-hover)] disabled:opacity-50">{submitting ? 'Creating...' : 'Create organization'}</button></div>{error && <p role="alert" className="mt-3 text-xs text-[var(--critical)]">{error}</p>}</div></div></form>}

        {organizations.length > 0 ? (
          <section aria-label="Organizations" className="mt-8 grid gap-4 md:grid-cols-2 xl:grid-cols-3">
            {organizations.map(({ organization, members, projects }, index) => (
              <article key={organization.id} className="group relative flex min-h-72 flex-col overflow-hidden rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--card)] p-5 transition-all duration-200 hover:-translate-y-0.5 hover:border-[var(--text-3)] hover:shadow-[0_18px_45px_rgba(0,0,0,0.24)]">
                <span className="absolute right-4 top-2 font-mono text-5xl font-semibold tracking-[-0.08em] text-[var(--accent)] opacity-10">{String(index + 1).padStart(2, '0')}</span>
                <div className="flex items-start justify-between gap-4">
                  <span className="grid h-10 w-10 place-items-center rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--panel)] font-mono text-[10px] font-semibold uppercase text-[var(--accent)]">
                    {organization.name.slice(0, 2)}
                  </span>
                </div>
                <h2 className="mt-5 truncate text-lg font-semibold tracking-[-0.025em]">{organization.name}</h2>
                <p className="mt-1 truncate font-mono text-[10px] text-[var(--text-3)]">{organization.slug}</p>
                <div className="mt-5 flex items-center gap-4 border-y border-[var(--border-soft)] py-3 text-[10px] uppercase tracking-[0.1em] text-[var(--text-3)]"><span className="inline-flex items-center gap-1.5"><Users aria-hidden="true" className="h-3.5 w-3.5" />{members.length} {members.length === 1 ? 'member' : 'members'}</span><span className="inline-flex items-center gap-1.5"><FolderKanban aria-hidden="true" className="h-3.5 w-3.5" />{projects.length} {projects.length === 1 ? 'project' : 'projects'}</span></div>
                <div className="mt-4 min-h-12"><p className="text-[9px] uppercase tracking-[0.14em] text-[var(--text-3)]">Project registry</p><div className="mt-2 flex flex-wrap gap-1.5">{projects.slice(0, 3).map((project) => <Link key={project.id} to="/projects/$projectId" params={{ projectId: project.id }} className="rounded-[4px] border border-[var(--border)] bg-[var(--panel)] px-2 py-1 font-mono text-[10px] text-[var(--text-2)] hover:border-[var(--accent)] hover:text-[var(--text)]">{project.name}</Link>)}{projects.length === 0 && <span className="text-xs text-[var(--text-3)]">No projects provisioned</span>}{projects.length > 3 && <span className="px-1 py-1 font-mono text-[10px] text-[var(--text-3)]">+{projects.length - 3}</span>}</div></div>
                <div className="mt-auto flex items-center justify-between gap-4 pt-5">
                  <Link to="/projects/new" search={{ organizationId: organization.id }} className="inline-flex items-center gap-1.5 text-xs font-medium text-[var(--text-2)] hover:text-[var(--accent)]"><Plus aria-hidden="true" className="h-3.5 w-3.5" />Add project</Link>
                  <Link to="/settings" search={{ organizationId: organization.id }} className="inline-flex items-center gap-1.5 text-xs font-medium text-[var(--accent)]">Manage<ArrowUpRight aria-hidden="true" className="h-3.5 w-3.5 transition-transform group-hover:-translate-y-0.5 group-hover:translate-x-0.5" /></Link>
                </div>
              </article>
            ))}
          </section>
        ) : (
          <section className="mt-5 rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--card)] p-8">
            <h2 className="text-base font-semibold">No organizations available</h2>
            <p className="mt-2 text-sm text-[var(--text-2)]">Your account is not currently associated with an organization.</p>
          </section>
        )}
      </div>
    </main>
  )
}

function OrgMetric({ value, label }: { value: number; label: string }) {
  return <div className="bg-[var(--panel)] px-4 py-3 sm:min-w-32"><strong className="block font-mono text-base font-medium text-[var(--accent)]">{value}</strong><span className="mt-0.5 block text-[9px] uppercase tracking-[0.12em] text-[var(--text-3)]">{label}</span></div>
}
