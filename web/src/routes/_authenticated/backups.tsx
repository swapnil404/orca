import { createFileRoute } from '@tanstack/react-router'
import { PlaceholderPage } from '../../components/shell/PlaceholderPage'

export const Route = createFileRoute('/_authenticated/backups')({
  component: BackupsPage,
})

function BackupsPage() {
  return <PlaceholderPage eyebrow="Recovery operations" title="Backups" description="A focused operational view for pgBackRest schedules, restore points, and recovery readiness across projects." features={[
    { title: 'Backup inventory', description: 'See available full, differential, and incremental backups for every cluster.' },
    { title: 'Recovery points', description: 'Understand retention coverage and the latest point-in-time recovery boundary.' },
    { title: 'Restore workflow', description: 'Prepare and track recovery operations with explicit status at every step.' },
  ]} />
}
