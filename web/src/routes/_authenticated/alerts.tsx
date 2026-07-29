import { createFileRoute } from '@tanstack/react-router'
import { PlaceholderPage } from '../../components/shell/PlaceholderPage'

export const Route = createFileRoute('/_authenticated/alerts')({
  component: AlertsPage,
})

function AlertsPage() {
  return <PlaceholderPage eyebrow="Incident intelligence" title="Alerts" description="Turn agent health reports into clear, actionable signals without coupling alert evaluation to ingestion." features={[
    { title: 'Health rules', description: 'Define thresholds for cluster health, replication lag, and stale agent reports.' },
    { title: 'Incident timeline', description: 'Review when a condition started, changed, and returned to a healthy state.' },
    { title: 'Delivery channels', description: 'Route notifications to the people and systems responsible for each project.' },
  ]} />
}
