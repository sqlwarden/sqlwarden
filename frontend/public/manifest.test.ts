import { existsSync, readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const dir = dirname(fileURLToPath(import.meta.url))
const manifestPath = join(dir, 'manifest.json')
const manifest = JSON.parse(readFileSync(manifestPath, 'utf-8')) as {
  short_name: string
  name: string
  theme_color: string
  background_color: string
  icons: Array<{ src: string; sizes: string; type: string }>
}

describe('manifest.json', () => {
  it('identifies the product as SQLWarden instead of the scaffold placeholder', () => {
    expect(manifest.short_name).toBe('SQLWarden')
    expect(manifest.name).toBe('SQLWarden')
  })

  it('uses the brand primary and light-surface colors', () => {
    expect(manifest.theme_color).toBe('#006EDC')
    expect(manifest.background_color).toBe('#F1F4F9')
  })

  it('references the regenerated favicon and logo assets', () => {
    expect(manifest.icons.map((icon) => icon.src)).toEqual([
      'favicon.ico',
      'logo192.png',
      'logo512.png',
    ])
  })

  it('ships every icon file it references', () => {
    for (const icon of manifest.icons) {
      expect(existsSync(join(dir, icon.src))).toBe(true)
    }
  })
})
