import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { BrandLockup } from './BrandLockup'

describe('BrandLockup', () => {
  it('renders the brand mark and wordmark together', () => {
    const { container } = render(<BrandLockup />)
    expect(container.querySelector('svg')).not.toBeNull()
    expect(screen.getByText('sqlwarden')).toBeInTheDocument()
  })

  it('forwards size to the mark and className to the wrapper', () => {
    const { container } = render(<BrandLockup size={20} className="text-lg" />)
    const svg = container.querySelector('svg')
    expect(svg).toHaveAttribute('width', '20')
    expect(container.firstElementChild).toHaveClass('text-lg')
  })
})
