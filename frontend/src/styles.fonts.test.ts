import { existsSync, readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const dir = dirname(fileURLToPath(import.meta.url))
const stylesPath = join(dir, 'styles.css')
const css = readFileSync(stylesPath, 'utf-8')
const fontsDir = join(dir, '..', 'public', 'fonts', 'satoshi')

describe('Satoshi self-hosting', () => {
  it('declares @font-face rules for the three self-hosted weights', () => {
    expect(css).toContain("font-family: 'Satoshi';")
    expect(css).toContain("url('/fonts/satoshi/Satoshi-Regular.woff2') format('woff2')")
    expect(css).toContain("url('/fonts/satoshi/Satoshi-Medium.woff2') format('woff2')")
    expect(css).toContain("url('/fonts/satoshi/Satoshi-Bold.woff2') format('woff2')")
    expect(css).toContain('font-display: swap;')
  })

  it('ships the woff2 files referenced by the @font-face rules', () => {
    expect(existsSync(join(fontsDir, 'Satoshi-Regular.woff2'))).toBe(true)
    expect(existsSync(join(fontsDir, 'Satoshi-Medium.woff2'))).toBe(true)
    expect(existsSync(join(fontsDir, 'Satoshi-Bold.woff2'))).toBe(true)
  })

  it('makes Satoshi the default interface font stack', () => {
    expect(css).toContain("--font-interface: 'Satoshi', 'Geist Variable', system-ui, sans-serif;")
  })
})
