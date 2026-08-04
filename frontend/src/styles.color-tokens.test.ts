import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const stylesPath = join(dirname(fileURLToPath(import.meta.url)), 'styles.css')
const css = readFileSync(stylesPath, 'utf-8')

function block(selector: string): string {
  const start = css.indexOf(`${selector} {`)
  if (start === -1) throw new Error(`selector "${selector}" not found in styles.css`)
  const end = css.indexOf('\n}', start)
  return css.slice(start, end)
}

describe('brand color tokens', () => {
  it('sets the brand primary blue in :root', () => {
    expect(block(':root')).toContain('--primary: #006EDC;')
  })

  it('lightens primary for AA contrast in dark mode', () => {
    expect(block('.dark')).toContain('--primary: #007FFE;')
  })

  it('defines the new semantic feedback tokens', () => {
    expect(block(':root')).toContain('--success: #10B981;')
    expect(block(':root')).toContain('--warning: #F59E0B;')
  })

  it('defines the new IDE accent tokens', () => {
    expect(block(':root')).toContain('--accent-link: #38BDF8;')
    expect(block(':root')).toContain('--accent-datatype: #6366F1;')
  })

  it('maps the new tokens through the Tailwind theme', () => {
    expect(css).toContain('--color-success: var(--success);')
    expect(css).toContain('--color-warning: var(--warning);')
    expect(css).toContain('--color-accent-link: var(--accent-link);')
    expect(css).toContain('--color-accent-datatype: var(--accent-datatype);')
  })
})
