import { AnimatePresence, motion, useReducedMotion } from 'framer-motion'
import { X } from 'lucide-react'
import { useEffect, useState, type ReactNode } from 'react'

interface DrawerProps {
  open: boolean
  onClose: () => void
  children: ReactNode
}

const premiumEase = [0.16, 1, 0.3, 1] as const
const defaultDurations = { base: 0.16, slow: 0.2 }

function durationInSeconds(value: string, fallback: number): number {
  const duration = Number.parseFloat(value)
  if (!Number.isFinite(duration)) return fallback
  return value.trim().endsWith('ms') ? duration / 1_000 : duration
}

export function Drawer({ open, onClose, children }: DrawerProps) {
  const [mounted, setMounted] = useState(false)
  const [durations, setDurations] = useState(defaultDurations)
  const reduceMotion = useReducedMotion()

  useEffect(() => {
    setMounted(true)
  }, [])

  useEffect(() => {
    if (!mounted) return

    const styles = window.getComputedStyle(document.documentElement)
    setDurations({
      base: durationInSeconds(styles.getPropertyValue('--dur-base'), defaultDurations.base),
      slow: durationInSeconds(styles.getPropertyValue('--dur-slow'), defaultDurations.slow),
    })
  }, [mounted])

  useEffect(() => {
    if (!mounted || !open) return

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [mounted, onClose, open])

  return (
    <AnimatePresence>
      {mounted && open ? (
        <motion.div
          className="fixed inset-0 z-50 flex justify-end"
          style={{ backgroundColor: 'rgba(0,0,0,0.4)' }}
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          transition={{ duration: durations.base }}
          onClick={onClose}
        >
          <motion.div
            role="dialog"
            aria-modal="true"
            aria-label="Detail drawer"
            className="relative h-full w-[min(440px,calc(100%-24px))] overflow-y-auto border border-white/[0.08] p-6 shadow-[-20px_0_60px_rgba(0,0,0,0.35)]"
            style={{
              backgroundColor: 'rgba(20,20,23,0.7)',
              backdropFilter: 'blur(20px)',
              WebkitBackdropFilter: 'blur(20px)',
            }}
            initial={{ opacity: 0, x: reduceMotion ? 0 : 24 }}
            animate={{ opacity: 1, x: 0 }}
            exit={{ opacity: 0, x: reduceMotion ? 0 : 24 }}
            transition={{ duration: durations.slow, ease: premiumEase }}
            onClick={(event) => event.stopPropagation()}
          >
            <button
              type="button"
              aria-label="Close drawer"
              onClick={onClose}
              className="absolute right-4 top-4 grid h-9 w-9 place-items-center rounded-full border border-white/[0.08] text-[var(--text-2)] transition-colors duration-[var(--dur-fast)] hover:bg-white/5 hover:text-[var(--text)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]"
            >
              <X aria-hidden="true" className="h-4 w-4" />
            </button>
            {children}
          </motion.div>
        </motion.div>
      ) : null}
    </AnimatePresence>
  )
}
