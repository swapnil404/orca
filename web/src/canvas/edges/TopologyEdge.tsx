import { EdgeLabelRenderer, getSmoothStepPath, type Edge, type EdgeProps } from '@xyflow/react'
import { motion, useReducedMotion } from 'framer-motion'

export type LagTone = 'healthy' | 'warning' | 'critical' | 'unknown'

export interface TopologyEdgeData extends Record<string, unknown> {
  lagLabel?: string
  lagTone?: LagTone
}

export type TopologyEdgeType = Edge<TopologyEdgeData, 'topology'>

const toneColor: Record<LagTone, string> = {
  healthy: 'var(--healthy)',
  warning: 'var(--warning)',
  critical: 'var(--critical)',
  unknown: 'var(--text-3)',
}

const toneDuration: Record<LagTone, number> = {
  healthy: 1.8,
  warning: 1.1,
  critical: 0.65,
  unknown: 0,
}

export function TopologyEdge({ sourceX, sourceY, targetX, targetY, sourcePosition, targetPosition, data, style }: EdgeProps<TopologyEdgeType>) {
  const reduceMotion = useReducedMotion()
  const [path, labelX, labelY] = getSmoothStepPath({ sourceX, sourceY, targetX, targetY, sourcePosition, targetPosition, borderRadius: 12 })
  const tone = data?.lagTone ?? 'unknown'
  const color = data?.lagLabel ? toneColor[tone] : 'var(--border)'
  const animated = Boolean(data?.lagLabel) && tone !== 'unknown'

  return (
    <>
      <motion.path
        d={path}
        fill="none"
        className="react-flow__edge-path"
        style={{
          ...style,
          stroke: color,
          strokeWidth: 1.5,
          strokeDasharray: data?.lagLabel ? '6 7' : undefined,
        }}
        initial={false}
        animate={{ strokeDashoffset: animated && !reduceMotion ? [0, -26] : 0 }}
        transition={{ duration: toneDuration[tone], repeat: animated && !reduceMotion ? Infinity : 0, ease: 'linear' }}
      />
      <circle cx={sourceX} cy={sourceY} r="3" fill={color} />
      <circle cx={targetX} cy={targetY} r="3" fill={color} />
      {data?.lagLabel ? (
        <EdgeLabelRenderer>
          <span
            className="nodrag nopan pointer-events-none absolute rounded-full border bg-[var(--panel)] px-2 py-1 font-mono text-[9px] shadow-sm"
            style={{ transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)`, borderColor: color, color }}
          >
            {data.lagLabel}
          </span>
        </EdgeLabelRenderer>
      ) : null}
    </>
  )
}
