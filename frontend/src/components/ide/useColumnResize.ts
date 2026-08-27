import { useEffect, useRef, useState } from 'react'

type ResizeState = {
  columnIndex: number
  startWidth: number
  startX: number
}

// Session-lifetime cache so widths a user drags survive switching away from a
// result tab and back — ResultsArea remounts ResultEntry on every tab/run
// change, so this can't live in component state. Keyed by column shape
// (names joined) rather than tab/run id, so the same query re-run in a new
// run keeps its widths too.
//
// Capped LRU: a long session running many distinct ad-hoc queries would
// otherwise grow this map forever. A Map iterates in insertion order, so
// re-inserting an entry on every touch keeps it at the "most recent" end
// and lets the oldest entry be evicted with a single `.next()` call.
const MAX_CACHE_ENTRIES = 200
const widthCache = new Map<string, number[]>()

function cacheGet(key: string): number[] | undefined {
  const value = widthCache.get(key)
  if (value) {
    widthCache.delete(key)
    widthCache.set(key, value)
  }
  return value
}

function cacheSet(key: string, value: number[]): void {
  widthCache.delete(key)
  widthCache.set(key, value)
  if (widthCache.size > MAX_CACHE_ENTRIES) {
    const oldestKey = widthCache.keys().next().value
    if (oldestKey !== undefined) widthCache.delete(oldestKey)
  }
}

function resolveDefaults(defaultWidth: number | number[], columnCount: number): number[] {
  return Array.isArray(defaultWidth)
    ? defaultWidth
    : Array.from({ length: columnCount }, () => defaultWidth)
}

export function useColumnResize(
  columnCount: number,
  defaultWidth: number | number[],
  minimumWidth: number,
  cacheKey?: string,
) {
  const [columnWidths, setColumnWidths] = useState(() => {
    const cached = cacheKey ? cacheGet(cacheKey) : undefined
    return cached && cached.length === columnCount
      ? cached
      : resolveDefaults(defaultWidth, columnCount)
  })
  const resizingRef = useRef<ResizeState | null>(null)
  const cleanupRef = useRef<(() => void) | null>(null)

  useEffect(() => () => cleanupRef.current?.(), [])

  function startResize(event: React.MouseEvent, columnIndex: number, measuredWidth?: number) {
    cleanupRef.current?.()
    event.preventDefault()
    resizingRef.current = {
      columnIndex,
      startX: event.clientX,
      startWidth: measuredWidth ?? columnWidths[columnIndex] ?? minimumWidth,
    }
    document.body.style.cursor = 'col-resize'
    document.body.style.userSelect = 'none'

    function cleanup() {
      resizingRef.current = null
      document.body.style.cursor = ''
      document.body.style.userSelect = ''
      window.removeEventListener('mousemove', handleMouseMove)
      window.removeEventListener('mouseup', cleanup)
      cleanupRef.current = null
    }

    function handleMouseMove(moveEvent: MouseEvent) {
      const resizing = resizingRef.current
      if (!resizing) {
        return
      }

      const width = Math.max(
        minimumWidth,
        resizing.startWidth + moveEvent.clientX - resizing.startX,
      )
      setColumnWidths((current) => {
        const next = [...current]
        next[resizing.columnIndex] = width
        if (cacheKey) cacheSet(cacheKey, next)
        return next
      })
    }

    cleanupRef.current = cleanup
    window.addEventListener('mousemove', handleMouseMove)
    window.addEventListener('mouseup', cleanup)
  }

  return { columnWidths, startResize }
}
