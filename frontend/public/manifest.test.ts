import { existsSync, readFileSync } from 'node:fs'
import { createHash } from 'node:crypto'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const dir = dirname(fileURLToPath(import.meta.url))
const manifestPath = join(dir, 'manifest.json')
const indexHtml = readFileSync(join(dir, '..', 'index.html'), 'utf-8')
const manifest = JSON.parse(readFileSync(manifestPath, 'utf-8')) as {
  short_name: string
  name: string
  theme_color: string
  background_color: string
  icons: Array<{ src: string; sizes: string; type: string; purpose?: string }>
}

describe('manifest.json', () => {
  it('identifies the product as SQLWarden instead of the scaffold placeholder', () => {
    expect(manifest.short_name).toBe('SQLWarden')
    expect(manifest.name).toBe('SQLWarden')
  })

  it('uses the mono icon background for installed-app surfaces', () => {
    expect(manifest.theme_color).toBe('#0A0A0A')
    expect(manifest.background_color).toBe('#0A0A0A')
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

  it('provides maskable PNG icons for installed apps', () => {
    expect(manifest.icons.filter((icon) => icon.type === 'image/png')).toEqual([
      expect.objectContaining({ sizes: '192x192', purpose: 'any maskable' }),
      expect.objectContaining({ sizes: '512x512', purpose: 'any maskable' }),
    ])
  })

  it('advertises only mono browser and Apple icon sources', () => {
    expect(indexHtml).toContain('content="#0A0A0A"')
    expect(indexHtml).toContain('href="/favicon.ico?v=4"')
    expect(indexHtml).toContain('href="/favicon.svg?v=4"')
    expect(indexHtml).toContain('href="/apple-touch-icon.png?v=4"')
    expect(indexHtml).not.toContain('rel="icon" type="image/png"')
    expect(existsSync(join(dir, 'apple-touch-icon.png'))).toBe(true)
  })

  it('keeps every published icon on the approved mono asset set', () => {
    const expectedHashes = {
      'favicon.ico': '828c952bd7214d1cd9312c19c0858272e617af1be77c93a706aac80e3cba55c8',
      'favicon.svg': '7c4c1d3e705dbb546f6f6a8bb1e8245e483e18ed6bb624e797138fc8b7a6d061',
      'apple-touch-icon.png': '6a32cf2d172d065d62fe973119ad422c177a655bede54d06927d33fc84a8724e',
      'logo192.png': '669ec7814084e536855981693b084f41b47b95ebc805b902c067ba1bd50f4430',
      'logo512.png': '558fcbdc8b6ee417f1994200e34572157dd97cb36cd3b5bf850e8eddc6deea3f',
    }

    for (const [filename, expectedHash] of Object.entries(expectedHashes)) {
      const hash = createHash('sha256')
        .update(readFileSync(join(dir, filename)))
        .digest('hex')
      expect(hash, filename).toBe(expectedHash)
    }
  })
})
