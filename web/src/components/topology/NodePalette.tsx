import { Archive, Blocks, ChevronDown, Database, GripVertical, Network } from 'lucide-react'
import { useState, type DragEvent } from 'react'
import { PALETTE_DRAG_TYPE, type PaletteNodeType } from './types'

interface NodePaletteProps {
  onActivate: (type: PaletteNodeType) => void
}

interface PaletteItem {
  type: PaletteNodeType
  name: string
  description: string
  icon: typeof Database
  color: string
}

interface PaletteSection {
  id: string
  label: string
  items: PaletteItem[]
}

const sections: PaletteSection[] = [
  { id: 'databases', label: 'Databases', items: [{ type: 'replica', name: 'Replica', description: 'Add a streaming read replica', icon: Database, color: 'text-[var(--streaming)]' }] },
  { id: 'poolers', label: 'Connection Poolers', items: [{ type: 'pgbouncer', name: 'PgBouncer', description: 'Provision a PostgreSQL pooler', icon: Network, color: 'text-[#bd9cff]' }] },
  { id: 'backups', label: 'Backup Tools', items: [{ type: 'pgbackrest', name: 'pgBackRest', description: 'Configure repository and schedule', icon: Archive, color: 'text-[var(--warning)]' }] },
  { id: 'extensions', label: 'Extensions', items: [{ type: 'extension', name: 'Extension', description: 'Install a reported capability', icon: Blocks, color: 'text-[var(--accent)]' }] },
]

export function NodePalette({ onActivate }: NodePaletteProps) {
  const [collapsed, setCollapsed] = useState<Set<string>>(() => new Set())

  function toggle(sectionID: string) {
    setCollapsed((current) => {
      const next = new Set(current)
      if (next.has(sectionID)) next.delete(sectionID)
      else next.add(sectionID)
      return next
    })
  }

  function startDrag(event: DragEvent<HTMLButtonElement>, type: PaletteNodeType) {
    event.dataTransfer.setData(PALETTE_DRAG_TYPE, type)
    event.dataTransfer.effectAllowed = 'copy'
  }

  return (
    <aside className="w-full shrink-0 rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--panel)] p-3 lg:w-64">
      <div className="border-b border-[var(--border-soft)] px-1 pb-3">
        <p className="font-mono text-[10px] font-medium uppercase tracking-[0.16em] text-[var(--text-3)]">Build topology</p>
        <p className="mt-1.5 text-xs leading-5 text-[var(--text-2)]">Drag desired resources onto the canvas.</p>
      </div>
      <div className="mt-2 space-y-1">
        {sections.map((section) => {
          const isCollapsed = collapsed.has(section.id)
          return <section key={section.id}>
            <button type="button" aria-expanded={!isCollapsed} onClick={() => toggle(section.id)} className="flex w-full items-center justify-between rounded-[var(--radius-sm)] px-1 py-2 text-left font-mono text-[10px] uppercase tracking-[0.12em] text-[var(--text-3)] hover:text-[var(--text-2)]">
              {section.label}<ChevronDown className={`h-3.5 w-3.5 transition-transform ${isCollapsed ? '-rotate-90' : ''}`} />
            </button>
            {!isCollapsed ? <div className="space-y-1.5 pb-2">{section.items.map((item) => {
              const Icon = item.icon
              return <button key={item.type} type="button" draggable onDragStart={(event) => startDrag(event, item.type)} onClick={() => onActivate(item.type)} className="group flex w-full cursor-grab items-center gap-3 rounded-[var(--radius-md)] border border-[var(--border-soft)] bg-[var(--card)] px-3 py-2.5 text-left hover:border-[var(--border)] active:cursor-grabbing">
                <span className="grid h-8 w-8 shrink-0 place-items-center rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--panel)]"><Icon className={`h-4 w-4 ${item.color}`} /></span>
                <span className="min-w-0 flex-1"><span className="block text-xs font-medium text-[var(--text)]">{item.name}</span><span className="mt-0.5 block truncate text-[10px] text-[var(--text-3)]">{item.description}</span></span>
                <GripVertical className="h-4 w-4 shrink-0 text-[var(--text-3)] opacity-0 transition-opacity group-hover:opacity-100" />
              </button>
            })}</div> : null}
          </section>
        })}
      </div>
    </aside>
  )
}
