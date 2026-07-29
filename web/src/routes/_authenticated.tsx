import { Outlet, createFileRoute, redirect, useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { getSession, logout } from '../api'
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
    <>
      <TopBar session={session} loggingOut={loggingOut} onLogout={handleLogout} />
      <Outlet />
    </>
  )
}
