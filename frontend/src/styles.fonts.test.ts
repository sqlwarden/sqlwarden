import { existsSync, readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const dir = dirname(fileURLToPath(import.meta.url))
const stylesPath = join(dir, 'styles.css')
const css = readFileSync(stylesPath, 'utf-8')
const fontsDir = join(dir, '..', 'public', 'fonts', 'satoshi')
const calSansUiDir = join(dir, '..', 'public', 'fonts', 'cal-sans-ui')
const calSansHeadingDir = join(dir, '..', 'public', 'fonts', 'cal-sans-heading')
const paperMonoDir = join(dir, '..', 'public', 'fonts', 'paper-mono')

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
})

describe('Cal Sans / Paper Mono self-hosting', () => {
  it('declares @font-face rules for Cal Sans UI, Cal Sans Heading, and Paper Mono', () => {
    expect(css).toContain("font-family: 'Cal Sans UI';")
    expect(css).toContain("url('/fonts/cal-sans-ui/CalSansUI-Variable.woff2') format('woff2')")
    expect(css).toContain("font-family: 'Cal Sans Heading';")
    expect(css).toContain("url('/fonts/cal-sans-heading/CalSans-SemiBold.woff2') format('woff2')")
    expect(css).toContain("font-family: 'Paper Mono';")
    expect(css).toContain("url('/fonts/paper-mono/PaperMono-Regular.woff2') format('woff2')")
  })

  it('ships the woff2 files referenced by the @font-face rules', () => {
    expect(existsSync(join(calSansUiDir, 'CalSansUI-Variable.woff2'))).toBe(true)
    expect(existsSync(join(calSansHeadingDir, 'CalSans-SemiBold.woff2'))).toBe(true)
    expect(existsSync(join(paperMonoDir, 'PaperMono-Regular.woff2'))).toBe(true)
  })

  it('makes Inter the default interface font stack', () => {
    expect(css).toContain("--font-interface: 'Inter Variable', system-ui, sans-serif;")
  })

  it('leaves Satoshi as the default heading font stack', () => {
    expect(css).toContain(
      "--font-heading-face: 'Satoshi', 'Geist Variable', system-ui, sans-serif;",
    )
  })

  it('makes JetBrains Mono the default data font stack', () => {
    expect(css).toContain("--font-data: 'JetBrains Mono Variable', ui-monospace, monospace;")
  })

  it('routes heading text through the heading font picker slot', () => {
    expect(css).toContain('--font-heading: var(--font-heading-face);')
  })
})
