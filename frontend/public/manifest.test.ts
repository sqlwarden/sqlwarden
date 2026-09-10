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

  it('keeps installed-app chrome on the neutral application background', () => {
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

  it('advertises the branded browser and Apple icon sources with neutral browser chrome', () => {
    expect(indexHtml).toContain('content="#FFFFFF" media="(prefers-color-scheme: light)"')
    expect(indexHtml).toContain('content="#0A0A0A" media="(prefers-color-scheme: dark)"')
    expect(indexHtml).toContain('href="/favicon.ico?v=5"')
    expect(indexHtml).toContain('href="/favicon.svg?v=5"')
    expect(indexHtml).toContain('href="/apple-touch-icon.png?v=5"')
    expect(indexHtml).not.toContain('rel="icon" type="image/png"')
    expect(existsSync(join(dir, 'apple-touch-icon.png'))).toBe(true)
  })

  it('keeps every published icon on the approved brand asset set', () => {
    const expectedHashes = {
      'favicon.ico': '3e917685c4edfc9852bfbf36ac0c61b292ced53fafa8dd0120ed224e8aec29a5',
      'favicon.svg': '99b6f7b102acc9b79e3520cef7ce63c4f6805c9a5adbb8c75b36043e9e7cc747',
      'apple-touch-icon.png': 'd0f708425416e7150e52b4fd7c5c87813108e63f3e6e8731cb1e1288b05715fc',
      'logo192.png': '437a19b1a6d441b47b307be741f9e4bb2496ee34a73c26e51745b79ccad28b39',
      'logo512.png': '9b7f8aa9985ea0cce9ce9013ae324bf155e24b24da62ca815cd0544d183abd47',
    }

    for (const [filename, expectedHash] of Object.entries(expectedHashes)) {
      const hash = createHash('sha256')
        .update(readFileSync(join(dir, filename)))
        .digest('hex')
      expect(hash, filename).toBe(expectedHash)
    }
  })
})
