import { motion, useReducedMotion } from 'framer-motion'
import type { ReactNode } from 'react'

interface BrushHighlightProps {
  children: ReactNode
  className?: string
}

export function BrushHighlight({ children, className = '' }: BrushHighlightProps) {
  const reduceMotion = useReducedMotion()

  return (
    <span className={`relative isolate inline-block px-[0.16em] text-[var(--accent-contrast)] ${className}`}>
      <span className="relative">{children}</span>
      <motion.svg
        aria-hidden="true"
        viewBox="0 0 320 72"
        preserveAspectRatio="none"
        initial={{ opacity: 0, scaleX: reduceMotion ? 1 : 0.05, rotate: reduceMotion ? 0 : -1.5 }}
        animate={{ opacity: 1, scaleX: 1, rotate: -0.5 }}
        transition={{ duration: reduceMotion ? 0.01 : 0.65, delay: reduceMotion ? 0 : 0.1, ease: [0.22, 1, 0.36, 1] }}
        style={{ transformOrigin: 'left center' }}
        className="absolute -inset-x-[0.12em] -inset-y-[0.02em] -z-10 h-[1.08em] w-[calc(100%+0.24em)] overflow-visible text-[var(--accent)]"
      >
        <path fill="currentColor" d="M5 22C52 9 111 11 162 8c58-3 113-6 150 7 8 3 8 28 0 34-37 18-98 13-151 17-55 4-113 7-154-6-9-3-10-33-2-38Z" />
      </motion.svg>
    </span>
  )
}
