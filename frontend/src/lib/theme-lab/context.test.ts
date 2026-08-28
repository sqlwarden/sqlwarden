import { describe, expect, it } from 'vitest'
import { tokenContrastRatio } from '#/lib/color-contrast'
import { SURFACE_PRESETS, surfaceTokens } from './context'

const AA_TEXT = 4.5

describe('surfaceTokens contrast', () => {
  for (const preset of SURFACE_PRESETS) {
    for (const isDark of [false, true]) {
      const mode = isDark ? 'dark' : 'light'

      it(`keeps "${preset.label}" (${mode}) body text at or above 4.5:1`, () => {
        const tokens = surfaceTokens(preset, isDark)
        expect(
          tokenContrastRatio(tokens['--background'], tokens['--foreground']),
        ).toBeGreaterThanOrEqual(AA_TEXT)
      })

      it(`keeps "${preset.label}" (${mode}) muted text at or above 4.5:1`, () => {
        const tokens = surfaceTokens(preset, isDark)
        expect(
          tokenContrastRatio(tokens['--muted'], tokens['--muted-foreground']),
        ).toBeGreaterThanOrEqual(AA_TEXT)
        expect(
          tokenContrastRatio(tokens['--background'], tokens['--muted-foreground']),
        ).toBeGreaterThanOrEqual(AA_TEXT)
      })
    }
  }
})
