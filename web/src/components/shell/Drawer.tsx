import { AnimatePresence, motion, useReducedMotion } from 'framer-motion'
import { useEffect, type ReactNode } from 'react'

interface DrawerProps {
  open: boolean
  onClose: () => void
  children: ReactNode
}

export function Drawer({ open, onClose, children }: DrawerProps) {
  const reduceMotion = useReducedMotion()

  useEffect(() => {
    if (!open) return

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [onClose, open])

  return (
    <AnimatePresence>
      {open ? (
        <motion.div
          className="fixed inset-0 z-50 flex justify-end bg-black/45"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          transition={{ duration: 0.16 }}
          onClick={onClose}
        >
          <motion.div
            role="dialog"
            aria-modal="true"
            aria-label="Infrastructure detail"
            className="h-full w-[min(440px,calc(100%-20px))] overflow-y-auto border-l border-[var(--border)] bg-[color:rgba(17,17,18,0.92)] p-5 shadow-[-24px_0_72px_rgba(0,0,0,0.38)] backdrop-blur-xl sm:p-6"
            initial={{ opacity: 0, x: reduceMotion ? 0 : 28 }}
            animate={{ opacity: 1, x: 0 }}
            exit={{ opacity: 0, x: reduceMotion ? 0 : 28 }}
            transition={{ duration: reduceMotion ? 0.01 : 0.2, ease: [0.16, 1, 0.3, 1] }}
            onClick={(event) => event.stopPropagation()}
          >
            {children}
          </motion.div>
        </motion.div>
      ) : null}
    </AnimatePresence>
  )
}
