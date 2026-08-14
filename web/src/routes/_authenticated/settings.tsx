import { createFileRoute, useNavigate, useRouter } from '@tanstack/react-router'
import { AlertTriangle, Trash2 } from 'lucide-react'
import { useEffect, useState, type FormEvent } from 'react'
import { ApiError, deleteOrganization, listOrganizationMembers, updateOrganization } from '../../api'
import type { Organization, OrganizationMember, OrganizationRole } from '../../types/organizations'

const tabs = ['Profile', 'Members', 'Secrets', 'Certificates', 'Tokens'] as const
type SettingsTab = (typeof tabs)[number]

interface SettingsData {
  organizations: Organization[]
  members: Record<string, OrganizationMember[]>
  userID: string
}

interface SettingsSearch {
  organizationId?: string
}

async function loadSettings(organizations: Organization[], userID: string): Promise<SettingsData> {
  const memberLists = await Promise.all(organizations.map(async (organization) => (
    [organization.id, await listOrganizationMembers(organization.id)] as const
  )))
  return { organizations, members: Object.fromEntries(memberLists), userID }
}

export const Route = createFileRoute('/_authenticated/settings')({
  ssr: false,
  validateSearch: (search: Record<string, unknown>): SettingsSearch => ({
    organizationId: typeof search.organizationId === 'string' ? search.organizationId : undefined,
  }),
  loader: ({ context }) => loadSettings(context.organizations, context.session.user_id),
  component: SettingsPage,
})

function SettingsPage() {
  const { organizations, members, userID } = Route.useLoaderData()
  const search = Route.useSearch()
  const navigate = useNavigate()
  const [activeTab, setActiveTab] = useState<SettingsTab>('Profile')
  const organizationID = organizations.some((item) => item.id === search.organizationId) ? search.organizationId ?? '' : organizations[0]?.id ?? ''
  const organization = organizations.find((item) => item.id === organizationID)
  const organizationMembers = organization ? members[organization.id] ?? [] : []
  const owner = organizationMembers.some((member) => member.user_id === userID && member.role === 'owner')

  return (
    <main className="min-h-[calc(100vh-56px)] px-4 py-6 text-[var(--text)] sm:px-6 lg:px-8">
      <div className="mx-auto max-w-5xl">
        <header className="flex flex-col justify-between gap-4 sm:flex-row sm:items-end">
          <div>
            <p className="text-xs font-medium uppercase tracking-[0.14em] text-[var(--text-3)]">Organization administration</p>
            <h1 className="mt-1 text-xl font-semibold">Settings</h1>
            <p className="mt-1 text-sm text-[var(--text-2)]">Manage organization identity, access, and credentials.</p>
          </div>
          {organizations.length > 1 && (
            <label className="text-xs font-medium text-[var(--text-2)]">
              Organization
              <select
                className="mt-1 block min-w-56 rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--card)] px-3 py-2 text-sm text-[var(--text)] outline-none focus:border-[var(--accent)]"
                value={organizationID}
                onChange={(event) => void navigate({ to: '/settings', search: { organizationId: event.target.value }, replace: true })}
              >
                {organizations.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
              </select>
            </label>
          )}
        </header>

        <div className="mt-6 overflow-x-auto border-b border-[var(--border)]" role="tablist" aria-label="Organization settings">
          <div className="flex min-w-max gap-6">
            {tabs.map((tab) => (
              <button
                key={tab}
                type="button"
                role="tab"
                aria-selected={activeTab === tab}
                className={`border-b-2 px-0.5 pb-3 text-sm font-medium transition-colors ${activeTab === tab ? 'border-[var(--accent)] text-[var(--text)]' : 'border-transparent text-[var(--text-2)] hover:text-[var(--text)]'}`}
                onClick={() => setActiveTab(tab)}
              >
                {tab}
              </button>
            ))}
          </div>
        </div>

        {!organization ? (
          <section className="mt-6 rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--card)] p-8">
            <h2 className="text-base font-semibold">No organization available</h2>
            <p className="mt-2 text-sm text-[var(--text-2)]">Your account is not currently associated with an organization.</p>
          </section>
        ) : (
          <div className="mt-6" role="tabpanel">
            {activeTab === 'Profile' && <ProfileSection organization={organization} owner={owner} />}
            {activeTab === 'Members' && <MembersSection members={organizationMembers} />}
            {activeTab === 'Secrets' && <UnavailableSection title="Organization secrets" description="Secret management is not available in the control-plane API yet." />}
            {activeTab === 'Certificates' && <UnavailableSection title="Certificates" description="Certificate listing and rotation status are not available in the control-plane API yet." />}
            {activeTab === 'Tokens' && <UnavailableSection title="API and CLI tokens" description="Token creation and revocation are not available in the control-plane API yet." />}
          </div>
        )}
      </div>
    </main>
  )
}

interface ProfileSectionProps {
  organization: Organization
  owner: boolean
}

function ProfileSection({ organization, owner }: ProfileSectionProps) {
  const navigate = useNavigate()
  const router = useRouter()
  const [name, setName] = useState(organization.name)
  const [saving, setSaving] = useState(false)
  const [message, setMessage] = useState('')
  const [confirmation, setConfirmation] = useState('')
  const [deleting, setDeleting] = useState(false)
  const [deleteError, setDeleteError] = useState('')
  useEffect(() => {
    setName(organization.name)
    setMessage('')
    setConfirmation('')
    setDeleteError('')
  }, [organization.id])

  async function handleSave(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const nextName = name.trim()
    if (!nextName || nextName === organization.name) return
    setSaving(true)
    setMessage('')
    try {
      const updated = await updateOrganization(organization.id, nextName)
      setName(updated.name)
      await router.invalidate()
      setMessage('Organization name updated.')
    } catch (cause) {
      setMessage(cause instanceof ApiError ? cause.message : 'Could not update the organization.')
    } finally {
      setSaving(false)
    }
  }

  async function handleDelete() {
    if (confirmation !== organization.name) return
    setDeleting(true)
    setDeleteError('')
    try {
      await deleteOrganization(organization.id)
      await navigate({ to: '/organizations' })
      await router.invalidate()
    } catch (cause) {
      setDeleteError(cause instanceof ApiError ? cause.message : 'Could not delete the organization.')
      setDeleting(false)
    }
  }

  return (
    <div className="space-y-6">
      <section className="rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--card)]">
        <div className="border-b border-[var(--border-soft)] px-5 py-4"><h2 className="text-base font-semibold">Organization profile</h2><p className="mt-1 text-sm text-[var(--text-2)]">Identity details used across this control plane.</p></div>
        <form onSubmit={handleSave} className="px-5 py-5"><label className="text-xs font-medium text-[var(--text-2)]">Name<input value={name} onChange={(event) => setName(event.target.value)} disabled={!owner || saving} required className="mt-2 w-full rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--panel)] px-3 py-2.5 text-sm text-[var(--text)] outline-none focus:border-[var(--accent)] disabled:opacity-50" /></label><dl className="mt-4 divide-y divide-[var(--border-soft)] border-y border-[var(--border-soft)]"><ProfileValue label="Slug" value={organization.slug} monospace /><ProfileValue label="Organization ID" value={organization.id} monospace /></dl><div className="mt-4 flex flex-wrap items-center justify-between gap-3"><p role="status" className="text-xs text-[var(--text-3)]">{message || (owner ? 'The slug remains stable when the name changes.' : 'Only organization owners can edit this profile.')}</p>{owner && <button disabled={saving || !name.trim() || name.trim() === organization.name} className="rounded-[var(--radius-sm)] bg-[var(--accent)] px-4 py-2.5 text-sm font-semibold text-[var(--accent-contrast)] hover:bg-[var(--accent-hover)] disabled:opacity-50">{saving ? 'Saving...' : 'Save changes'}</button>}</div></form>
      </section>
      {owner && <section className="overflow-hidden rounded-[var(--radius-sm)] border border-[var(--critical)]/35 bg-[var(--card)]"><div className="flex gap-3 border-b border-[var(--critical)]/25 bg-[var(--critical)]/5 px-5 py-4"><AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-[var(--critical)]" /><div><h2 className="font-medium text-[var(--critical)]">Delete organization</h2><p className="mt-1 text-xs leading-5 text-[var(--text-2)]">Deletion is permanent and is only available when the organization has no project history.</p></div></div><div className="px-5 py-5"><label className="text-xs font-medium text-[var(--text-2)]">Type <span className="font-mono text-[var(--text)]">{organization.name}</span> to confirm<input value={confirmation} onChange={(event) => setConfirmation(event.target.value)} autoComplete="off" className="mt-2 w-full rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--panel)] px-3 py-2.5 text-sm text-[var(--text)] outline-none focus:border-[var(--critical)]" /></label>{deleteError && <p role="alert" className="mt-3 text-xs text-[var(--critical)]">{deleteError}</p>}<div className="mt-4 flex justify-end"><button type="button" onClick={handleDelete} disabled={deleting || confirmation !== organization.name} className="inline-flex items-center gap-2 rounded-[var(--radius-sm)] bg-[var(--critical)] px-4 py-2.5 text-sm font-semibold text-white hover:brightness-110 disabled:opacity-40"><Trash2 className="h-4 w-4" />{deleting ? 'Deleting...' : 'Delete organization'}</button></div></div></section>}
    </div>
  )
}

interface ProfileValueProps {
  label: string
  value: string
  monospace?: boolean
}

function ProfileValue({ label, value, monospace = false }: ProfileValueProps) {
  return (
    <div className="grid gap-1 py-4 sm:grid-cols-[180px_1fr] sm:items-center">
      <dt className="text-sm font-medium text-[var(--text-2)]">{label}</dt>
      <dd className={`break-all text-sm ${monospace ? 'font-mono text-xs' : ''}`}>{value}</dd>
    </div>
  )
}

interface MembersSectionProps {
  members: OrganizationMember[]
}

function MembersSection({ members }: MembersSectionProps) {
  return (
    <section className="overflow-hidden rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--card)]">
      <div className="flex items-center justify-between border-b border-[var(--border-soft)] px-5 py-4">
        <div>
          <h2 className="text-base font-semibold">Members</h2>
          <p className="mt-1 text-sm text-[var(--text-2)]">People with access to this organization.</p>
        </div>
        <span className="font-mono text-xs text-[var(--text-3)]">{members.length} total</span>
      </div>
      {members.length > 0 ? (
        <div className="overflow-x-auto">
          <table className="w-full min-w-[620px] border-collapse text-left text-sm">
            <thead className="bg-[var(--panel)] text-xs font-medium text-[var(--text-2)]">
              <tr><th className="px-5 py-3">Member</th><th className="px-5 py-3">Role</th><th className="px-5 py-3">Joined</th></tr>
            </thead>
            <tbody className="divide-y divide-[var(--border-soft)]">
              {members.map((member) => (
                <tr key={member.id}>
                  <td className="px-5 py-3">
                    <div className="font-medium">{member.email || 'Member'}</div>
                    <div className="mt-0.5 font-mono text-[11px] text-[var(--text-3)]">{member.user_id}</div>
                  </td>
                  <td className="px-5 py-3"><RoleBadge role={member.role} /></td>
                  <td className="px-5 py-3 font-mono text-xs text-[var(--text-2)]">{new Date(member.created_at).toLocaleDateString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <p className="px-5 py-8 text-sm text-[var(--text-2)]">No active members were returned for this organization.</p>
      )}
      <div className="border-t border-[var(--border-soft)] bg-[var(--panel)] px-5 py-3 text-xs text-[var(--text-2)]">Member invitations are not available yet.</div>
    </section>
  )
}

function RoleBadge({ role }: { role: OrganizationRole }) {
  return <span className="inline-flex rounded-full border border-[var(--border)] bg-[var(--panel)] px-2.5 py-1 text-xs font-medium capitalize">{role}</span>
}

interface UnavailableSectionProps {
  title: string
  description: string
}

function UnavailableSection({ title, description }: UnavailableSectionProps) {
  return (
    <section className="rounded-[var(--radius-sm)] border border-dashed border-[var(--border)] bg-[var(--card)] px-6 py-10 text-center">
      <p className="text-xs font-medium uppercase tracking-[0.14em] text-[var(--text-3)]">Coming soon</p>
      <h2 className="mt-2 text-base font-semibold">{title}</h2>
      <p className="mx-auto mt-2 max-w-lg text-sm text-[var(--text-2)]">{description}</p>
    </section>
  )
}
