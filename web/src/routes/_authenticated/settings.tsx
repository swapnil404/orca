import { createFileRoute } from '@tanstack/react-router'
import { PlaceholderPage } from '../../components/shell/PlaceholderPage'

export const Route = createFileRoute('/_authenticated/settings')({
  component: SettingsPage,
})

function SettingsPage() {
  return <PlaceholderPage eyebrow="Workspace control" title="Settings" description="Manage control-plane preferences, authentication, and project-level defaults from one predictable workspace." features={[
    { title: 'Workspace', description: 'Manage display details and defaults shared across your control plane.' },
    { title: 'Access', description: 'Review active authentication methods and workspace access policies.' },
    { title: 'Agent defaults', description: 'Set safe defaults for future project and agent registrations.' },
  ]} />
}
