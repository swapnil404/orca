import { AnimatePresence, motion, useReducedMotion } from 'framer-motion'
import { useEffect, useRef, type ReactNode } from 'react'

const focusableSelector = [
  'a[href]',
  'area[href]',
  'button:not([disabled])',
  'input:not([disabled])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  'iframe',
  '[contenteditable]:not([contenteditable="false"])',
  '[tabindex]:not([tabindex="-1"])',
].join(',')

function getFocusableElements(container: HTMLElement): HTMLElement[] {
  return [...container.querySelectorAll<HTMLElement>(focusableSelector)]
    .filter((element) => !element.hidden && element.getAttribute('aria-hidden') !== 'true' && element.getClientRects().length > 0)
}

function inertBackground(drawer: HTMLElement): () => void {
  const previousStates = new Map<HTMLElement, boolean>()
  let current: HTMLElement = drawer

  while (current.parentElement) {
    for (const sibling of current.parentElement.children) {
      if (!(sibling instanceof HTMLElement) || sibling === current) continue
      previousStates.set(sibling, sibling.inert)
      sibling.inert = true
    }
    current = current.parentElement
  }

  return () => {
    for (const [element, wasInert] of previousStates) element.inert = wasInert
  }
}

interface DrawerProps {
  open: boolean
  onClose: () => void
  children: ReactNode
}

export function Drawer({ open, onClose, children }: DrawerProps) {
  const reduceMotion = useReducedMotion()
  const drawerRef = useRef<HTMLDivElement>(null)
  const onCloseRef = useRef(onClose)
  onCloseRef.current = onClose

  useEffect(() => {
    if (!open) return

    const drawer = drawerRef.current
    if (!drawer) return

    const previouslyFocused = document.activeElement instanceof HTMLElement ? document.activeElement : null
    const restoreBackground = inertBackground(drawer)
    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'

    const focusFrame = requestAnimationFrame(() => {
      const focusTarget = getFocusableElements(drawer)[0] ?? drawer
      focusTarget.focus()
    })

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        onCloseRef.current()
        return
      }
      if (event.key !== 'Tab') return

      const focusableElements = getFocusableElements(drawer)
      if (focusableElements.length === 0) {
        event.preventDefault()
        drawer.focus()
        return
      }

      const firstElement = focusableElements[0]
      const lastElement = focusableElements[focusableElements.length - 1]
      if (event.shiftKey && (document.activeElement === firstElement || !drawer.contains(document.activeElement))) {
        event.preventDefault()
        lastElement.focus()
      } else if (!event.shiftKey && (document.activeElement === lastElement || !drawer.contains(document.activeElement))) {
        event.preventDefault()
        firstElement.focus()
      }
    }
    document.addEventListener('keydown', handleKeyDown)
    return () => {
      cancelAnimationFrame(focusFrame)
      document.removeEventListener('keydown', handleKeyDown)
      restoreBackground()
      document.body.style.overflow = previousOverflow
      if (previouslyFocused?.isConnected) previouslyFocused.focus()
    }
  }, [open])

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
            ref={drawerRef}
            role="dialog"
            aria-modal="true"
            aria-label="Infrastructure detail"
            tabIndex={-1}
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
