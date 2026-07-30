import { Outlet, createFileRoute, redirect, useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { getSession, logout } from '../api'
import { CommandPalette } from '../components/shell/CommandPalette'
import { TopBar } from '../components/shell/TopBar'

export const Route = createFileRoute('/_authenticated')({
  beforeLoad: async () => {
    const session = await getSession()
    if (!session) {
      throw redirect({ to: '/login' })
    }
    return { session }
  },
  component: AuthenticatedLayout,
})

function AuthenticatedLayout() {
  const navigate = useNavigate()
  const { session } = Route.useRouteContext()
  const [loggingOut, setLoggingOut] = useState(false)

  async function handleLogout() {
    setLoggingOut(true)
    await logout()
    await navigate({ to: '/login' })
  }

  return (
    <div className="min-h-screen pb-14 pt-14 sm:pb-0 sm:pl-24 sm:pt-0">
      <TopBar session={session} loggingOut={loggingOut} onLogout={handleLogout} />
      <CommandPalette />
      <Outlet />
    </div>
  )
}
