import { Link, createFileRoute } from '@tanstack/react-router'
import { Building2, Users } from 'lucide-react'
import { listOrganizationMembers, listOrganizations } from '../../api'
import type { Organization, OrganizationMember } from '../../types/organizations'

interface OrganizationSummary {
  organization: Organization
  members: OrganizationMember[]
}

async function loadOrganizations(): Promise<OrganizationSummary[]> {
  const organizations = await listOrganizations()
  return Promise.all(organizations.map(async (organization) => ({
    organization,
    members: await listOrganizationMembers(organization.id),
  })))
}

export const Route = createFileRoute('/_authenticated/organizations')({
  ssr: false,
  loader: loadOrganizations,
  component: OrganizationsPage,
})

function OrganizationsPage() {
  const organizations = Route.useLoaderData()
  const memberCount = organizations.reduce((total, item) => total + item.members.length, 0)

  return (
    <main className="min-h-[calc(100vh-56px)] px-4 py-6 text-[var(--text)] sm:px-6 lg:px-8">
      <div className="mx-auto max-w-7xl">
        <header className="flex flex-col justify-between gap-3 sm:flex-row sm:items-center">
          <div>
            <p className="font-mono text-[10px] font-medium uppercase tracking-[0.16em] text-[var(--text-3)]">Orca home</p>
            <h1 className="mt-1.5 text-xl font-semibold">Organizations</h1>
            <p className="mt-1 text-sm text-[var(--text-2)]">Workspaces and memberships available to your account.</p>
          </div>
          <div className="flex divide-x divide-[var(--border)] rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--panel)] text-xs">
            <span className="px-3 py-2"><strong className="mr-1.5 font-mono text-[var(--text)]">{organizations.length}</strong><span className="text-[var(--text-2)]">Organizations</span></span>
            <span className="px-3 py-2"><strong className="mr-1.5 font-mono text-[var(--text)]">{memberCount}</strong><span className="text-[var(--text-2)]">Memberships</span></span>
          </div>
        </header>

        {organizations.length > 0 ? (
          <section aria-label="Organizations" className="mt-5 grid gap-3 md:grid-cols-2 xl:grid-cols-3">
            {organizations.map(({ organization, members }) => (
              <article key={organization.id} className="flex min-h-52 flex-col rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--card)] p-5">
                <div className="flex items-start justify-between gap-4">
                  <span className="grid h-9 w-9 place-items-center rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--panel)] text-[var(--accent)]">
                    <Building2 aria-hidden="true" className="h-4 w-4" strokeWidth={1.7} />
                  </span>
                  <span className="inline-flex items-center gap-1.5 text-xs text-[var(--text-2)]"><Users aria-hidden="true" className="h-3.5 w-3.5" />{members.length} {members.length === 1 ? 'member' : 'members'}</span>
                </div>
                <h2 className="mt-5 truncate text-base font-semibold">{organization.name}</h2>
                <p className="mt-1 font-mono text-xs text-[var(--text-3)]">{organization.slug}</p>
                <div className="mt-auto flex items-end justify-between gap-4 border-t border-[var(--border-soft)] pt-4">
                  <div><p className="text-[10px] uppercase tracking-[0.12em] text-[var(--text-3)]">Created</p><p className="mt-1 font-mono text-xs text-[var(--text-2)]">{new Date(organization.created_at).toLocaleDateString()}</p></div>
                  <Link to="/settings" className="text-xs font-medium text-[var(--accent)] hover:underline">Manage organization</Link>
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
