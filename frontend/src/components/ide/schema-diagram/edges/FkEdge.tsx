import { BaseEdge, getBezierPath, getSmoothStepPath, Position, type EdgeProps } from '@xyflow/react'

export type FkMarker = 'one' | 'many'
export type FkEdgeData = {
  sourceMarker: FkMarker
  targetMarker: FkMarker
  // Orthogonal routing shows crow's-foot cardinality markers; bezier (curved)
  // routing omits them (the curve's tangent doesn't align with the markers).
  orthogonal: boolean
}

const DIST = 14 // marker distance from the node along the edge
const SPREAD = 5 // half-height of crow's-foot / bar

// Direction the edge leaves a handle, along x (our handles are always Left/Right).
function dirX(pos: Position): number {
  if (pos === Position.Left) return -1
  if (pos === Position.Right) return 1
  return 0
}

function markerPath(kind: FkMarker, x: number, y: number, pos: Position): string {
  const d = dirX(pos)
  const px = x + d * DIST
  if (kind === 'one') {
    // A single bar perpendicular to the edge ("exactly one").
    return `M ${px} ${y - SPREAD} L ${px} ${y + SPREAD}`
  }
  // Crow's foot ("many"): three prongs at the node fanning to a point on the edge.
  return `M ${px} ${y} L ${x} ${y - SPREAD} M ${px} ${y} L ${x} ${y} M ${px} ${y} L ${x} ${y + SPREAD}`
}

export function FkEdge({
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
  style,
  data,
  markerEnd,
}: EdgeProps) {
  const d = data as unknown as FkEdgeData | undefined
  const orthogonal = Boolean(d?.orthogonal)
  const params = { sourceX, sourceY, sourcePosition, targetX, targetY, targetPosition }
  const [path] = orthogonal ? getSmoothStepPath(params) : getBezierPath(params)
  const stroke = (style?.stroke as string | undefined) ?? 'var(--muted-foreground)'
  const opacity = style?.opacity as number | undefined
  return (
    <>
      <BaseEdge path={path} style={style} markerEnd={markerEnd} />
      {d && orthogonal && (
        <g
          stroke={stroke}
          strokeWidth={1.2}
          fill="none"
          opacity={opacity}
          style={{ pointerEvents: 'none' }}
        >
          <path d={markerPath(d.sourceMarker, sourceX, sourceY, sourcePosition)} />
          <path d={markerPath(d.targetMarker, targetX, targetY, targetPosition)} />
        </g>
      )}
    </>
  )
}
