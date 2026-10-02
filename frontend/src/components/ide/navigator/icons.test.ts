import { describe, expect, it } from 'vitest'
import { NAVIGATOR_ICONS, navigatorIcon } from './icons'

describe('navigatorIcon', () => {
  it('resolves every registered token to its own style', () => {
    for (const [token, style] of Object.entries(NAVIGATOR_ICONS)) {
      expect(navigatorIcon(token, true)).toBe(style)
    }
  })

  it('falls back to a neutral icon for an unregistered token', () => {
    expect(navigatorIcon('not-a-token', true)).toEqual({
      icon: 'box',
      className: 'text-muted-foreground',
    })
  })

  it('drops the accent for objects nested under another object', () => {
    expect(navigatorIcon('trigger', false)).toEqual({
      icon: NAVIGATOR_ICONS.trigger.icon,
      className: 'text-muted-foreground',
    })
  })
})
