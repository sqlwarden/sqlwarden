import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { tokenContrastRatio } from '#/lib/color-contrast'

const stylesPath = join(dirname(fileURLToPath(import.meta.url)), 'styles.css')
const css = readFileSync(stylesPath, 'utf-8')

function block(selector: string): string {
  const start = css.indexOf(`${selector} {`)
  if (start === -1) throw new Error(`selector "${selector}" not found in styles.css`)
  const end = css.indexOf('\n}', start)
  return css.slice(start, end)
}

/** Reads a CSS custom property's raw value out of a selector block, e.g.
 *  `token(':root', '--background')` -> `'oklch(1 0 0)'`. Tests assert
 *  against the live file content, not hand-copied literals, so a token edit
 *  can't silently drift out of sync with its contrast-ratio coverage. */
function token(selector: string, name: string): string {
  const match = new RegExp(`${name}:\\s*([^;]+);`).exec(block(selector))
  if (!match) throw new Error(`token "${name}" not found in "${selector}" block`)
  return match[1].trim()
}

describe('brand color tokens', () => {
  it('sets the brand primary blue in :root', () => {
    expect(token(':root', '--primary')).toBe('oklch(0.55 0.19 255)')
  })

  it('lightens primary for AA contrast in dark mode', () => {
    expect(token('.dark', '--primary')).toBe('oklch(0.68 0.15 250)')
  })

  it('maps the new tokens through the Tailwind theme', () => {
    expect(css).toContain('--color-success: var(--success);')
    expect(css).toContain('--color-warning: var(--warning);')
    expect(css).toContain('--color-accent-link: var(--accent-link);')
    expect(css).toContain('--color-accent-datatype: var(--accent-datatype);')
  })
})

describe('WCAG contrast ratios', () => {
  const AA_TEXT = 4.5

  it('keeps light-mode muted text at or above 4.5:1 against muted and page background', () => {
    const mutedFg = token(':root', '--muted-foreground')
    expect(tokenContrastRatio(token(':root', '--muted'), mutedFg)).toBeGreaterThanOrEqual(AA_TEXT)
    expect(tokenContrastRatio(token(':root', '--background'), mutedFg)).toBeGreaterThanOrEqual(
      AA_TEXT,
    )
  })

  it('keeps dark-mode body and muted text at or above 4.5:1', () => {
    const bg = token('.dark', '--background')
    const fg = token('.dark', '--foreground')
    const muted = token('.dark', '--muted')
    const mutedFg = token('.dark', '--muted-foreground')
    expect(tokenContrastRatio(bg, fg)).toBeGreaterThanOrEqual(AA_TEXT)
    expect(tokenContrastRatio(muted, mutedFg)).toBeGreaterThanOrEqual(AA_TEXT)
    expect(tokenContrastRatio(bg, mutedFg)).toBeGreaterThanOrEqual(AA_TEXT)
  })

  it('keeps dark-mode body text below a harsh near-white/near-black contrast', () => {
    // Regression guard for the "high-contrast mode" look: body text should
    // land in the ~11-13:1 band editors like VS Code/Postman use, not the
    // ~17:1 a near-white foreground on a near-black background produces.
    const ratio = tokenContrastRatio(token('.dark', '--background'), token('.dark', '--foreground'))
    expect(ratio).toBeLessThan(14)
  })

  it('keeps feedback/accent tokens at or above 4.5:1 as text on their intended background', () => {
    const lightBg = token(':root', '--background')
    for (const name of ['--success', '--warning', '--accent-link', '--accent-datatype']) {
      expect(tokenContrastRatio(token(':root', name), lightBg)).toBeGreaterThanOrEqual(AA_TEXT)
    }
    expect(tokenContrastRatio(token(':root', '--destructive'), lightBg)).toBeGreaterThanOrEqual(
      AA_TEXT,
    )

    const darkBg = token('.dark', '--background')
    for (const name of ['--success', '--warning', '--accent-link', '--accent-datatype']) {
      expect(tokenContrastRatio(token('.dark', name), darkBg)).toBeGreaterThanOrEqual(AA_TEXT)
    }
  })
})
