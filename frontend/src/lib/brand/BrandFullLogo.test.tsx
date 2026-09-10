import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { BrandColorLockup, BrandFullLogo } from './BrandFullLogo'

describe('BrandFullLogo', () => {
  it('renders an inline svg with a height matching the size prop', () => {
    const { container } = render(<BrandFullLogo size={28} />)
    const svg = container.querySelector('svg')
    expect(svg).not.toBeNull()
    expect(svg).toHaveAttribute('height', '28')
  })

  it('scales width to preserve the logo aspect ratio', () => {
    const { container } = render(<BrandFullLogo size={16} />)
    const svg = container.querySelector('svg')
    expect(svg).toHaveAttribute('width', String((16 * 1448.1) / 279.9))
  })

  it('defaults to size 16 and forwards className', () => {
    const { container } = render(<BrandFullLogo className="text-foreground" />)
    const svg = container.querySelector('svg')
    expect(svg).toHaveAttribute('height', '16')
    expect(svg).toHaveClass('text-foreground')
  })

  it('inherits color via currentColor so it adapts to its text-color context', () => {
    const { container } = render(<BrandFullLogo />)
    const svg = container.querySelector('svg')
    expect(svg).toHaveAttribute('fill', 'currentColor')
  })
})

describe('BrandColorLockup', () => {
  it('renders the blue mark without a background and keeps the wordmark theme-aware', () => {
    const { container } = render(<BrandColorLockup size={28} />)

    expect(container.querySelector('rect')).toBeNull()
    expect(container.querySelector('[data-brand-mark]')).toHaveAttribute(
      'fill',
      'var(--brand-solid)',
    )
    expect(container.querySelector('g')).not.toHaveAttribute('fill')
    expect(container.querySelector('svg')).toHaveAttribute('fill', 'currentColor')
  })

  it('keeps the monochrome lockup entirely current-color', () => {
    const { container } = render(<BrandFullLogo />)

    expect(container.querySelector('rect')).toBeNull()
    expect(container.querySelector('[data-brand-mark]')).toHaveAttribute('fill', 'currentColor')
  })
})
