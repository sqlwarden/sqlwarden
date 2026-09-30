import { describe, expect, it } from 'vitest'
import { NAVIGATOR_ICONS, navigatorIcon } from './icons'

describe('navigatorIcon', () => {
  it('resolves every registered token to its own style', () => {
    for (const [token, style] of Object.entries(NAVIGATOR_ICONS)) {
      expect(navigatorIcon(token)).toBe(style)
    }
  })

  it('falls back to a neutral icon for an unregistered token', () => {
    expect(navigatorIcon('not-a-token')).toEqual({
      icon: 'box',
      className: 'text-muted-foreground',
    })
  })
})
