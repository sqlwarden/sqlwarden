import { useCallback, useEffect, useRef, useState, type WheelEvent } from 'react'

export function useTabStripOverflow(activeTabId: string | undefined, tabCount: number) {
  const scrollRef = useRef<HTMLDivElement>(null)
  const [canScrollLeft, setCanScrollLeft] = useState(false)
  const [canScrollRight, setCanScrollRight] = useState(false)

  const update = useCallback(() => {
    const element = scrollRef.current
    if (!element) return
    setCanScrollLeft(element.scrollLeft > 1)
    setCanScrollRight(element.scrollLeft + element.clientWidth < element.scrollWidth - 1)
  }, [])

  useEffect(() => {
    const element = scrollRef.current
    if (!element) return
    update()
    element.addEventListener('scroll', update, { passive: true })
    const observer = new ResizeObserver(update)
    observer.observe(element)
    return () => {
      element.removeEventListener('scroll', update)
      observer.disconnect()
    }
  }, [update])

  useEffect(() => {
    const element = scrollRef.current
    if (element && activeTabId) {
      element.querySelector(`[data-tab-id="${activeTabId}"]`)?.scrollIntoView({ inline: 'nearest', block: 'nearest' })
    }
    update()
  }, [activeTabId, tabCount, update])

  function scroll(direction: -1 | 1) {
    const element = scrollRef.current
    if (!element) return
    element.scrollBy({ left: direction * Math.max(160, element.clientWidth * 0.7), behavior: 'smooth' })
  }

  function handleWheel(event: WheelEvent<HTMLDivElement>) {
    const element = scrollRef.current
    if (!element || element.scrollWidth <= element.clientWidth) return
    const delta = Math.abs(event.deltaY) > Math.abs(event.deltaX) ? event.deltaY : event.deltaX
    element.scrollBy({ left: delta })
  }

  return { canScrollLeft, canScrollRight, handleWheel, scroll, scrollRef }
}
