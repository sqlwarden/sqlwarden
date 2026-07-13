import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { useTabStripOverflow } from './useTabStripOverflow'

function Strip({ active = 'tab-1', count = 2 }: { active?: string; count?: number }) {
  const overflow = useTabStripOverflow(active, count)
  return (
    <>
      <div ref={overflow.scrollRef} data-testid="strip" onWheel={overflow.handleWheel}>
        <span data-tab-id="tab-1" />
      </div>
      <output data-testid="state">{`${overflow.canScrollLeft}:${overflow.canScrollRight}`}</output>
      <button type="button" onClick={() => overflow.scroll(1)}>next</button>
    </>
  )
}

describe('useTabStripOverflow', () => {
  it('tracks horizontal overflow as the strip scrolls', async () => {
    render(<Strip />)
    const strip = screen.getByTestId('strip')
    Object.defineProperties(strip, {
      clientWidth: { configurable: true, value: 100 },
      scrollWidth: { configurable: true, value: 300 },
      scrollLeft: { configurable: true, writable: true, value: 0 },
    })

    fireEvent.scroll(strip)
    await waitFor(() => expect(screen.getByTestId('state')).toHaveTextContent('false:true'))

    strip.scrollLeft = 200
    fireEvent.scroll(strip)
    await waitFor(() => expect(screen.getByTestId('state')).toHaveTextContent('true:false'))
  })

  it('maps vertical wheel movement and chevrons to horizontal scrolling', () => {
    render(<Strip />)
    const strip = screen.getByTestId('strip')
    Object.defineProperties(strip, {
      clientWidth: { configurable: true, value: 100 },
      scrollWidth: { configurable: true, value: 300 },
    })
    const scrollBy = vi.fn()
    strip.scrollBy = scrollBy

    fireEvent.wheel(strip, { deltaX: 2, deltaY: 40 })
    expect(scrollBy).toHaveBeenCalledWith({ left: 40 })

    fireEvent.click(screen.getByRole('button', { name: 'next' }))
    expect(scrollBy).toHaveBeenCalledWith({ left: 160, behavior: 'smooth' })
  })
})
