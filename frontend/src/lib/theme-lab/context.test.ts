import { describe, expect, it } from 'vitest'
import { compositeOverBackground, contrastRatio, tokenContrastRatio } from '#/lib/color-contrast'
import { SURFACE_PRESETS, surfaceTokens } from './context'

const AA_TEXT = 4.5
const AAA_TEXT = 7
const NON_TEXT = 3
const MIN_TAB_LIGHTNESS_STEP = 0.025

function oklchLightness(token: string): number {
  const match = /^oklch\(([\d.]+)/.exec(token)
  if (!match) throw new Error(`not an oklch token: ${token}`)
  return Number(match[1])
}

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

      for (const [strip, content] of [
        ['--editor-tab-strip', '--editor'],
        ['--panel-tab-strip', '--panel'],
      ] as const) {
        it(`separates "${preset.label}" (${mode}) ${strip} from the ${content} surface its selected tab takes`, () => {
          const tokens = surfaceTokens(preset, isDark)
          expect(
            Math.abs(oklchLightness(tokens[strip]) - oklchLightness(tokens[content])),
          ).toBeGreaterThanOrEqual(MIN_TAB_LIGHTNESS_STEP)
        })

        it(`keeps "${preset.label}" (${mode}) unselected tab text on ${strip} at or above 4.5:1`, () => {
          const tokens = surfaceTokens(preset, isDark)
          expect(
            tokenContrastRatio(tokens[strip], tokens['--muted-foreground']),
          ).toBeGreaterThanOrEqual(AA_TEXT)
        })
      }
    }
  }
})

describe('"High Contrast" surface preset', () => {
  const preset = SURFACE_PRESETS.find((p) => p.id === 'high-contrast')
  if (!preset) throw new Error('"high-contrast" preset not found in SURFACE_PRESETS')

  for (const isDark of [false, true]) {
    const mode = isDark ? 'dark' : 'light'

    it(`clears WCAG AAA (7:1) body and muted text in ${mode} mode`, () => {
      const tokens = surfaceTokens(preset, isDark)
      expect(
        tokenContrastRatio(tokens['--background'], tokens['--foreground']),
      ).toBeGreaterThanOrEqual(AAA_TEXT)
      expect(
        tokenContrastRatio(tokens['--background'], tokens['--muted-foreground']),
      ).toBeGreaterThanOrEqual(AAA_TEXT)
    })

    it(`clears the 3:1 non-text floor for --border against --background in ${mode} mode`, () => {
      const tokens = surfaceTokens(preset, isDark)
      const border = compositeOverBackground(tokens['--border'], tokens['--background'])
      const background = compositeOverBackground(tokens['--background'], tokens['--background'])
      expect(contrastRatio(border, background)).toBeGreaterThanOrEqual(NON_TEXT)
    })
  }
})
