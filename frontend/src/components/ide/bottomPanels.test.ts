import { describe, expect, it } from 'vitest'
import { BOTTOM_PANELS, visibleBottomPanels } from './bottomPanels'

describe('visibleBottomPanels', () => {
  it('always includes Results', () => {
    const panels = visibleBottomPanels({ queryHistoryMode: 'off', queryFavoritesMode: 'off' })
    expect(panels.map((p) => p.id)).toEqual(['results'])
  })

  it('includes History and Favorites unless their mode is off', () => {
    const panels = visibleBottomPanels({ queryHistoryMode: 'backend', queryFavoritesMode: 'local' })
    expect(panels.map((p) => p.id)).toEqual(['results', 'history', 'favorites'])
  })

  it('gates History independently of Favorites', () => {
    const panels = visibleBottomPanels({ queryHistoryMode: 'off', queryFavoritesMode: 'backend' })
    expect(panels.map((p) => p.id)).toEqual(['results', 'favorites'])
  })

  it('registers Results, History, and Favorites in that order', () => {
    expect(BOTTOM_PANELS.map((p) => p.id)).toEqual(['results', 'history', 'favorites'])
  })
})
