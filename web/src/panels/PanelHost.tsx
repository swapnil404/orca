import { Drawer } from '../components/shell/Drawer'
import { AlertsPanel } from './AlertsPanel'
import { ClusterPanel } from './ClusterPanel'
import { ExtensionsPanel } from './ExtensionsPanel'
import { PgBackRestPanel } from './PgBackRestPanel'
import { PgBouncerPanel } from './PgBouncerPanel'
import { ReplicaPanel } from './ReplicaPanel'
import type { DetailSelection } from './types'

interface PanelHostProps {
  selected: DetailSelection | null
  onClose: () => void
}

export function PanelHost({ selected, onClose }: PanelHostProps) {
  let content = null
  if (selected?.kind === 'cluster') content = <ClusterPanel resource={selected} onClose={onClose} />
  if (selected?.kind === 'replica') content = <ReplicaPanel resource={selected} onClose={onClose} />
  if (selected?.kind === 'pgbouncer') content = <PgBouncerPanel resource={selected} onClose={onClose} />
  if (selected?.kind === 'pgbackrest') content = <PgBackRestPanel cluster={selected.cluster} actual={selected.state?.actual_state?.backup} onClose={onClose} />
  if (selected?.kind === 'extension') content = <ExtensionsPanel cluster={selected.cluster} onClose={onClose} />
  if (selected?.kind === 'extensions') content = <ExtensionsPanel cluster={selected.cluster} onClose={onClose} />
  if (selected?.kind === 'alerts') content = <AlertsPanel cluster={selected.cluster} onClose={onClose} />

  return <Drawer open={selected !== null} onClose={onClose}>{content}</Drawer>
}
