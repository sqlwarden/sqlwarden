/** WCAG 2.1 relative luminance / contrast ratio helpers, plus parsers for the
 *  `oklch(...)` and hex color strings used throughout styles.css and the
 *  Theme Lab. Kept dependency-free so tests can check real CSS variable
 *  values for accessibility regressions without a browser. */

export type Srgb = [number, number, number]

function oklchToSrgb(L: number, C: number, hDeg: number): Srgb {
  const h = (hDeg * Math.PI) / 180
  const a = C * Math.cos(h)
  const b = C * Math.sin(h)
  const l_ = L + 0.3963377774 * a + 0.2158037573 * b
  const m_ = L - 0.1055613458 * a - 0.0638541728 * b
  const s_ = L - 0.0894841775 * a - 1.291485548 * b
  const l = l_ ** 3
  const m = m_ ** 3
  const s = s_ ** 3
  const rLin = 4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s
  const gLin = -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s
  const bLin = -0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s
  const toSrgb = (c: number) => {
    const clamped = Math.min(1, Math.max(0, c))
    return clamped <= 0.0031308 ? 12.92 * clamped : 1.055 * clamped ** (1 / 2.4) - 0.055
  }
  return [toSrgb(rLin), toSrgb(gLin), toSrgb(bLin)]
}

function hexToSrgb(hex: string): Srgb {
  const h = hex.replace('#', '')
  const r = parseInt(h.slice(0, 2), 16) / 255
  const g = parseInt(h.slice(2, 4), 16) / 255
  const b = parseInt(h.slice(4, 6), 16) / 255
  return [r, g, b]
}

/** Parses a color as it appears in styles.css/Theme Lab tokens: either a
 *  hex literal (`#10b981`) or an `oklch(L C H)` / `oklch(L C H / A%)`
 *  expression. The alpha channel, if present, is ignored — callers that need
 *  a composited (over-background) color should blend separately. */
export function parseColorToSrgb(value: string): Srgb {
  const trimmed = value.trim()
  if (trimmed.startsWith('#')) return hexToSrgb(trimmed)

  const match = /oklch\(\s*([\d.]+)\s+([\d.]+)\s+([\d.]+)/.exec(trimmed)
  if (!match) throw new Error(`Unrecognized color value: "${value}"`)
  const [, l, c, h] = match
  return oklchToSrgb(Number(l), Number(c), Number(h))
}

function relativeLuminance([r, g, b]: Srgb): number {
  const lin = (c: number) => (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4)
  return 0.2126 * lin(r) + 0.7152 * lin(g) + 0.0722 * lin(b)
}

/** WCAG 2.1 contrast ratio between two sRGB colors, in the range [1, 21]. */
export function contrastRatio(a: Srgb, b: Srgb): number {
  const l1 = relativeLuminance(a)
  const l2 = relativeLuminance(b)
  const [hi, lo] = l1 > l2 ? [l1, l2] : [l2, l1]
  return (hi + 0.05) / (lo + 0.05)
}

/** Convenience: contrast ratio between two raw token values (hex or oklch). */
export function tokenContrastRatio(a: string, b: string): number {
  return contrastRatio(parseColorToSrgb(a), parseColorToSrgb(b))
}

function parseAlpha(value: string): number {
  const match = /\/\s*([\d.]+)%/.exec(value)
  return match ? Number(match[1]) / 100 : 1
}

/** Alpha-composites a token (e.g. a semi-transparent `--border`) over a solid
 *  background token, for checking non-text contrast (WCAG 1.4.11) of colors
 *  that are only visible blended with what's behind them. */
export function compositeOverBackground(foreground: string, background: string): Srgb {
  const alpha = parseAlpha(foreground)
  const fg = parseColorToSrgb(foreground)
  const bg = parseColorToSrgb(background)
  return [0, 1, 2].map((i) => bg[i] * (1 - alpha) + fg[i] * alpha) as Srgb
}
