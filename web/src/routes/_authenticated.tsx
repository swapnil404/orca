import { Outlet, createFileRoute, redirect, useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { getSession, listOrganizations, listProjects, logout } from '../api'
import { CommandPalette } from '../components/shell/CommandPalette'
import { TopBar } from '../components/shell/TopBar'

export const Route = createFileRoute('/_authenticated')({
  ssr: false,
  beforeLoad: async () => {
    const session = await getSession()
    if (!session) {
      throw redirect({ to: '/login' })
    }
    return { session }
  },
  loader: async () => {
    const [organizations, projects] = await Promise.all([listOrganizations(), listProjects()])
    return { organizations, projects }
  },
  component: AuthenticatedLayout,
})

function AuthenticatedLayout() {
  const navigate = useNavigate()
  const { session } = Route.useRouteContext()
  const { organizations, projects } = Route.useLoaderData()
  const [loggingOut, setLoggingOut] = useState(false)
  const [commandPaletteOpen, setCommandPaletteOpen] = useState(false)

  async function handleLogout() {
    setLoggingOut(true)
    await logout()
    await navigate({ to: '/login' })
  }

  return (
    <div className="min-h-screen pt-28 md:pl-16 md:pt-16">
      <TopBar session={session} organizations={organizations} projects={projects} loggingOut={loggingOut} onLogout={handleLogout} onOpenSearch={() => setCommandPaletteOpen(true)} />
      <CommandPalette open={commandPaletteOpen} onOpenChange={setCommandPaletteOpen} />
      <Outlet />
    </div>
  )
}
