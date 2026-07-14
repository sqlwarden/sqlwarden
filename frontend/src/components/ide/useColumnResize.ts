import { useEffect, useRef, useState } from 'react'

type ResizeState = {
  columnIndex: number
  startWidth: number
  startX: number
}

export function useColumnResize(columnCount: number, defaultWidth: number, minimumWidth: number) {
  const [columnWidths, setColumnWidths] = useState(() =>
    Array.from({ length: columnCount }, () => defaultWidth),
  )
  const resizingRef = useRef<ResizeState | null>(null)
  const cleanupRef = useRef<(() => void) | null>(null)

  useEffect(() => () => cleanupRef.current?.(), [])

  function startResize(event: React.MouseEvent, columnIndex: number) {
    cleanupRef.current?.()
    event.preventDefault()
    resizingRef.current = {
      columnIndex,
      startX: event.clientX,
      startWidth: columnWidths[columnIndex] ?? defaultWidth,
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
        return next
      })
    }

    cleanupRef.current = cleanup
    window.addEventListener('mousemove', handleMouseMove)
    window.addEventListener('mouseup', cleanup)
  }

  return { columnWidths, startResize }
}
