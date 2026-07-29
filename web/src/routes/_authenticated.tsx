import { Outlet, createFileRoute, redirect, useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { getSession, logout } from '../api'

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
  const [loggingOut, setLoggingOut] = useState(false)

  async function handleLogout() {
    setLoggingOut(true)
    await logout()
    await navigate({ to: '/login' })
  }

  return (
    <>
      <button type="button" disabled={loggingOut} onClick={handleLogout} className="fixed right-4 top-4 z-50 rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--card)] px-3 py-2 text-xs font-medium text-[var(--text-2)] shadow-lg transition hover:text-[var(--text)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)] disabled:opacity-60">
        {loggingOut ? 'Logging out...' : 'Log out'}
      </button>
      <Outlet />
    </>
  )
}
