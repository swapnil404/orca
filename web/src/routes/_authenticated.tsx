import { Outlet, createFileRoute, redirect, useNavigate, useRouterState } from '@tanstack/react-router'
import { AnimatePresence, motion, useReducedMotion } from 'framer-motion'
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
  const pathname = useRouterState({ select: (state) => state.location.pathname })
  const reduceMotion = useReducedMotion()

  async function handleLogout() {
    setLoggingOut(true)
    await logout()
    await navigate({ to: '/login' })
  }

  return (
    <div className="min-h-screen pb-16 pt-16 sm:pb-0 sm:pl-24 sm:pt-0">
      <TopBar session={session} loggingOut={loggingOut} onLogout={handleLogout} />
      <CommandPalette />
      <AnimatePresence mode="wait" initial={false}>
        <motion.div
          key={pathname}
          initial={{ opacity: 0, y: reduceMotion ? 0 : 10 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: reduceMotion ? 0 : -6 }}
          transition={{ duration: reduceMotion ? 0.01 : 0.32, ease: [0.22, 1, 0.36, 1] }}
        >
          <Outlet />
        </motion.div>
      </AnimatePresence>
    </div>
  )
}
