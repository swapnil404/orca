import type { Project } from '../types/resources'

export interface CommandDestination {
  to: '/' | '/alerts' | '/backups' | '/settings' | '/projects/$projectId'
  projectId?: string
}

export interface Command {
  id: string
  label: string
  description: string
  keywords: string[]
  destination: CommandDestination
}

export const staticCommands: Command[] = [
  { id: 'projects', label: 'Projects', description: 'View all projects', keywords: ['home', 'infrastructure'], destination: { to: '/' } },
  { id: 'alerts', label: 'Alerts', description: 'Review active alerts', keywords: ['incidents', 'warnings'], destination: { to: '/alerts' } },
  { id: 'backups', label: 'Backups', description: 'Review backup activity', keywords: ['restore', 'recovery'], destination: { to: '/backups' } },
  { id: 'settings', label: 'Settings', description: 'Configure the control plane', keywords: ['preferences', 'configuration'], destination: { to: '/settings' } },
  { id: 'organization', label: 'Organization', description: 'Manage workspace access', keywords: ['org', 'members', 'workspace'], destination: { to: '/settings' } },
]

export function projectCommands(projects: Project[]): Command[] {
  return projects.map((project) => ({
    id: `project-${project.id}`,
    label: project.name,
    description: 'Jump to project',
    keywords: ['project', project.id],
    destination: { to: '/projects/$projectId', projectId: project.id },
  }))
}

function scoreText(value: string, query: string): number | null {
  const text = value.toLocaleLowerCase()
  if (text === query) return 100
  if (text.startsWith(query)) return 80 - Math.min(text.length - query.length, 20)

  const wordIndex = text.split(/\s+/).findIndex((word) => word.startsWith(query))
  if (wordIndex >= 0) return 65 - wordIndex

  const substringIndex = text.indexOf(query)
  if (substringIndex >= 0) return 50 - Math.min(substringIndex, 20)

  let queryIndex = 0
  let spread = 0
  let lastMatch = -1
  for (let index = 0; index < text.length && queryIndex < query.length; index += 1) {
    if (text[index] !== query[queryIndex]) continue
    if (lastMatch >= 0) spread += index - lastMatch - 1
    lastMatch = index
    queryIndex += 1
  }

  return queryIndex === query.length ? 25 - Math.min(spread, 20) : null
}

export function filterCommands(commands: Command[], query: string): Command[] {
  const normalizedQuery = query.trim().toLocaleLowerCase()
  if (!normalizedQuery) return commands

  return commands
    .map((command, index) => {
      const scores = [command.label, ...command.keywords]
        .map((value) => scoreText(value, normalizedQuery))
        .filter((score): score is number => score !== null)
      return { command, index, score: scores.length > 0 ? Math.max(...scores) : null }
    })
    .filter((result): result is { command: Command; index: number; score: number } => result.score !== null)
    .sort((a, b) => b.score - a.score || a.index - b.index)
    .map(({ command }) => command)
}
