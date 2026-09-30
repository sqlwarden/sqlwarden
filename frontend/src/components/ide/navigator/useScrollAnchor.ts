import { useLayoutEffect, useState } from 'react'

function scrollParentOf(element: HTMLElement): HTMLElement | null {
  for (let node = element.parentElement; node; node = node.parentElement) {
    const { overflowY } = getComputedStyle(node)
    if (overflowY === 'auto' || overflowY === 'scroll') return node
  }
  return null
}

/**
 * Finds the nearest scrolling ancestor of `element` and its offset within that
 * ancestor's scrolled content. Callers nest the navigator inside a shared
 * scroll container (DatabasePanel lists several connections), so the
 * virtualizer needs this offset as its scroll margin; it is re-measured
 * whenever sibling content resizes.
 */
export function useScrollAnchor(element: HTMLElement | null) {
  const [scrollElement, setScrollElement] = useState<HTMLElement | null>(null)
  const [scrollMargin, setScrollMargin] = useState(0)

  useLayoutEffect(() => {
    if (!element) return
    const parent = scrollParentOf(element)
    setScrollElement(parent)
    if (!parent) return
    const measure = () =>
      setScrollMargin(
        element.getBoundingClientRect().top - parent.getBoundingClientRect().top + parent.scrollTop,
      )
    measure()
    if (typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(measure)
    for (const child of Array.from(parent.children)) observer.observe(child)
    return () => observer.disconnect()
  }, [element])

  return { scrollElement, scrollMargin }
}
