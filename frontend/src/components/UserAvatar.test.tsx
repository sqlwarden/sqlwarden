// @vitest-environment jsdom

import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { UserAvatar } from './UserAvatar'

describe('UserAvatar', () => {
  it('renders the same blobatar markup for the same identity', () => {
    const { container: first } = render(<UserAvatar value="ada@example.com" />)
    const { container: second } = render(<UserAvatar value="ada@example.com" />)

    expect(first.querySelector('img')?.getAttribute('src')).toBe(
      second.querySelector('img')?.getAttribute('src'),
    )
  })

  it('renders different blobatars for different identities', () => {
    const { container: ada } = render(<UserAvatar value="ada@example.com" />)
    const { container: grace } = render(<UserAvatar value="grace@example.com" />)

    expect(ada.querySelector('img')?.getAttribute('src')).not.toBe(
      grace.querySelector('img')?.getAttribute('src'),
    )
  })

  it('falls back to the fallback string as the seed when value is empty', () => {
    const { container: empty } = render(<UserAvatar value="" fallback="team-a" />)
    const { container: named } = render(<UserAvatar value="team-a" />)

    expect(empty.querySelector('img')?.getAttribute('src')).toBe(
      named.querySelector('img')?.getAttribute('src'),
    )
  })

  it('sizes the image in pixels for named presets and numeric overrides', () => {
    render(<UserAvatar value="ada@example.com" size="lg" />)
    expect(screen.getByRole('img')).toHaveAttribute('width', '40')

    render(<UserAvatar value="ada@example.com" size={64} />)
    expect(screen.getAllByRole('img')[1]).toHaveAttribute('width', '64')
  })

  it('uses the identity as the accessible title', () => {
    render(<UserAvatar value="Ada Lovelace" />)
    expect(screen.getByRole('img')).toHaveAttribute('alt', 'Ada Lovelace')
  })
})
