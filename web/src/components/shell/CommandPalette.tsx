import { useNavigate } from '@tanstack/react-router'
import { ArrowDown, ArrowUp, CornerDownLeft, Search } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { listProjects } from '../../api'
import { filterCommands, projectCommands, staticCommands, type Command } from '../../lib/commands'
import type { Project } from '../../types/resources'

function isMacPlatform(): boolean {
  return /Mac|iPhone|iPad|iPod/i.test(navigator.platform) || /Macintosh|Mac OS X|iPhone|iPad|iPod/i.test(navigator.userAgent)
}

function isFormFieldFocused(paletteInput: HTMLInputElement | null): boolean {
  const activeElement = document.activeElement
  if (!(activeElement instanceof Element) || activeElement === paletteInput) return false

  const tagName = activeElement.tagName
  return tagName === 'INPUT'
    || tagName === 'TEXTAREA'
    || tagName === 'SELECT'
    || activeElement.closest('[contenteditable]:not([contenteditable="false"]), [role="textbox"], [role="combobox"], [role="searchbox"]') !== null
}

interface CommandPaletteProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function CommandPalette({ open, onOpenChange }: CommandPaletteProps) {
  const navigate = useNavigate()
  const inputRef = useRef<HTMLInputElement>(null)
  const [mounted, setMounted] = useState(false)
  const [query, setQuery] = useState('')
  const [projects, setProjects] = useState<Project[]>([])
  const [loadingProjects, setLoadingProjects] = useState(false)
  const [projectsError, setProjectsError] = useState(false)
  const [selectedIndex, setSelectedIndex] = useState(0)
  const commands = filterCommands([...staticCommands, ...projectCommands(projects)], query)

  useEffect(() => {
    setMounted(true)
  }, [])

  useEffect(() => {
    if (!mounted) return

    const handleShortcut = (event: KeyboardEvent) => {
      if (event.key.toLocaleLowerCase() !== 'k' || isFormFieldFocused(inputRef.current)) return
      const shortcutPressed = isMacPlatform() ? event.metaKey : event.ctrlKey
      if (!shortcutPressed) return
      event.preventDefault()
      onOpenChange(!open)
    }

    document.addEventListener('keydown', handleShortcut)
    return () => document.removeEventListener('keydown', handleShortcut)
  }, [mounted, onOpenChange, open])

  useEffect(() => {
    if (!open) return

    let active = true
    setQuery('')
    setSelectedIndex(0)
    setLoadingProjects(true)
    setProjectsError(false)
    requestAnimationFrame(() => inputRef.current?.focus())

    void listProjects()
      .then((nextProjects) => {
        if (active) setProjects(nextProjects)
      })
      .catch(() => {
        if (active) setProjectsError(true)
      })
      .finally(() => {
        if (active) setLoadingProjects(false)
      })

    return () => {
      active = false
    }
  }, [open])

  useEffect(() => {
    if (!open) return

    const handleEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onOpenChange(false)
    }

    document.addEventListener('keydown', handleEscape)
    return () => document.removeEventListener('keydown', handleEscape)
  }, [onOpenChange, open])

  useEffect(() => {
    if (selectedIndex < commands.length) return
    setSelectedIndex(Math.max(0, commands.length - 1))
  }, [commands.length, selectedIndex])

  async function execute(command: Command) {
    onOpenChange(false)
    if (command.destination.to === '/projects/$projectId' && command.destination.projectId) {
      await navigate({ to: '/projects/$projectId', params: { projectId: command.destination.projectId } })
      return
    }
    await navigate({ to: command.destination.to })
  }

  function handleKeyDown(event: React.KeyboardEvent<HTMLInputElement>) {
    if (event.key === 'Escape') {
      event.preventDefault()
      onOpenChange(false)
      return
    }
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      setSelectedIndex((current) => commands.length === 0 ? 0 : (current + 1) % commands.length)
      return
    }
    if (event.key === 'ArrowUp') {
      event.preventDefault()
      setSelectedIndex((current) => commands.length === 0 ? 0 : (current - 1 + commands.length) % commands.length)
      return
    }
    if (event.key === 'Enter' && commands[selectedIndex]) {
      event.preventDefault()
      void execute(commands[selectedIndex])
    }
  }

  if (!mounted || !open) return null

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center bg-black/55 px-3 pt-[14vh]" onMouseDown={() => onOpenChange(false)}>
      <section
        role="dialog"
        aria-modal="true"
        aria-label="Command palette"
        className="relative w-full max-w-2xl overflow-hidden rounded-[var(--radius-lg)] border border-[var(--border)] bg-[color:rgba(17,17,18,0.96)] shadow-[0_28px_90px_rgba(0,0,0,0.65)] backdrop-blur-xl"
        onMouseDown={(event) => event.stopPropagation()}
      >
        <div aria-hidden="true" className="absolute inset-x-0 top-0 h-px bg-gradient-to-r from-transparent via-[var(--accent)] to-transparent opacity-80" />
        <div className="flex items-center gap-3 border-b border-[var(--border)] px-5">
          <Search aria-hidden="true" className="h-4 w-4 shrink-0 text-[var(--accent)]" />
          <input
            ref={inputRef}
            value={query}
            onChange={(event) => { setQuery(event.target.value); setSelectedIndex(0) }}
            onKeyDown={handleKeyDown}
            placeholder="Search projects, settings, or actions..."
            aria-label="Search commands"
            aria-controls="command-palette-results"
            aria-activedescendant={commands[selectedIndex] ? `command-${commands[selectedIndex].id}` : undefined}
            className="h-16 min-w-0 flex-1 bg-transparent text-sm text-[var(--text)] outline-none placeholder:text-[var(--text-3)]"
          />
          <kbd className="rounded-[4px] border border-[var(--border)] bg-[var(--card)] px-1.5 py-0.5 font-mono text-[9px] text-[var(--text-3)]">ESC</kbd>
        </div>

        <div id="command-palette-results" role="listbox" className="max-h-[min(420px,55vh)] overflow-y-auto p-2">
          {commands.map((command, index) => (
            <button
              id={`command-${command.id}`}
              key={command.id}
              type="button"
              role="option"
              aria-selected={index === selectedIndex}
              onMouseEnter={() => setSelectedIndex(index)}
              onClick={() => void execute(command)}
              className={`flex w-full items-center justify-between gap-4 rounded-xl px-3 py-2.5 text-left transition-colors ${index === selectedIndex ? 'bg-white/[0.08] text-[var(--text)]' : 'text-[var(--text-2)] hover:bg-white/[0.05]'}`}
            >
              <span className="min-w-0"><span className="block truncate text-sm font-medium">{command.label}</span><span className="block truncate text-xs text-[var(--text-3)]">{command.description}</span></span>
              {index === selectedIndex ? <CornerDownLeft aria-hidden="true" className="h-3.5 w-3.5 shrink-0 text-[var(--text-3)]" /> : null}
            </button>
          ))}
          {commands.length === 0 ? <p className="px-3 py-8 text-center text-sm text-[var(--text-3)]">No matching commands</p> : null}
          {loadingProjects ? <p className="px-3 py-2 text-xs text-[var(--text-3)]">Loading projects...</p> : null}
          {projectsError ? <p className="px-3 py-2 text-xs text-[var(--warning)]">Projects could not be loaded.</p> : null}
        </div>

        <footer className="flex items-center gap-3 border-t border-[var(--border)] bg-[var(--panel)] px-4 py-2 text-[10px] text-[var(--text-3)]">
          <span className="inline-flex items-center gap-1"><ArrowUp className="h-3 w-3" /><ArrowDown className="h-3 w-3" /> Navigate</span>
          <span className="inline-flex items-center gap-1"><CornerDownLeft className="h-3 w-3" /> Open</span>
        </footer>
      </section>
    </div>
  )
}
