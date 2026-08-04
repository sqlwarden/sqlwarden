import { createContext, useContext, useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { useTheme } from '#/components/theme-provider'

// ─── Accent ────────────────────────────────────────────────────────────────────

type Oklch = { l: number; c: number; h: number }

export type AccentPreset = {
  id: string
  label: string
  light: Oklch
  dark: Oklch
}

export const ACCENT_PRESETS: AccentPreset[] = [
  {
    // id kept stable for localStorage compatibility; label reflects that
    // this is the brand default — values match styles.css's baked-in
    // --primary (#006edc light / #007ffe dark) so the swatch and the
    // applied color agree when this preset is the active default.
    id: 'steel-teal',
    label: 'Default',
    light: { l: 0.551, c: 0.188, h: 256 },
    dark: { l: 0.612, c: 0.211, h: 256 },
  },
  {
    id: 'blue',
    label: 'Blue',
    light: { l: 0.55, c: 0.19, h: 255 },
    dark: { l: 0.68, c: 0.15, h: 250 },
  },
  {
    id: 'indigo',
    label: 'Indigo',
    light: { l: 0.545, c: 0.2, h: 272 },
    dark: { l: 0.68, c: 0.16, h: 270 },
  },
  {
    id: 'violet',
    label: 'Violet',
    light: { l: 0.545, c: 0.22, h: 292 },
    dark: { l: 0.68, c: 0.18, h: 292 },
  },
  {
    id: 'emerald',
    label: 'Emerald',
    light: { l: 0.55, c: 0.135, h: 160 },
    dark: { l: 0.72, c: 0.14, h: 160 },
  },
  {
    id: 'amber',
    label: 'Amber',
    light: { l: 0.66, c: 0.13, h: 65 },
    dark: { l: 0.78, c: 0.13, h: 70 },
  },
  {
    id: 'rose',
    label: 'Rose',
    light: { l: 0.59, c: 0.19, h: 15 },
    dark: { l: 0.7, c: 0.17, h: 15 },
  },
  {
    id: 'graphite',
    label: 'Graphite',
    light: { l: 0.35, c: 0.012, h: 240 },
    dark: { l: 0.8, c: 0.012, h: 240 },
  },
]

export type Accent = { kind: 'preset'; id: string } | { kind: 'custom'; hex: string }

const DEFAULT_ACCENT_ID = 'steel-teal'
export const DEFAULT_ACCENT: Accent = { kind: 'preset', id: DEFAULT_ACCENT_ID }

const oklchStr = ({ l, c, h }: Oklch) => `oklch(${l} ${c} ${h})`

/** Foreground that stays legible on the accent: dark ink on bright accents,
 *  near-white on deep ones. */
function accentForeground(lightness: number): string {
  return lightness > 0.62 ? 'oklch(0.16 0.015 240)' : 'oklch(0.985 0.005 240)'
}

/** sRGB hex → approximate OKLCH lightness (enough to pick a foreground). */
function hexLightness(hex: string): number {
  const m = /^#?([0-9a-f]{6})$/i.exec(hex.trim())
  if (!m) return 0.5
  const n = parseInt(m[1], 16)
  const srgb = [(n >> 16) & 255, (n >> 8) & 255, n & 255].map((v) => {
    const s = v / 255
    return s <= 0.04045 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4)
  })
  const y = 0.2126 * srgb[0] + 0.7152 * srgb[1] + 0.0722 * srgb[2]
  // Cube-root approximation of OKLab L from relative luminance.
  return Math.cbrt(y)
}

function accentColors(accent: Accent, isDark: boolean): { color: string; foreground: string } {
  if (accent.kind === 'custom') {
    return { color: accent.hex, foreground: accentForeground(hexLightness(accent.hex)) }
  }
  const preset = ACCENT_PRESETS.find((p) => p.id === accent.id) ?? ACCENT_PRESETS[0]
  const value = isDark ? preset.dark : preset.light
  return { color: oklchStr(value), foreground: accentForeground(value.l) }
}

const ACCENT_VAR_TARGETS = [
  '--primary',
  '--ring',
  '--sidebar-primary',
  '--sidebar-ring',
  '--chart-1',
] as const

// ─── Surface ───────────────────────────────────────────────────────────────────

export type SurfacePreset = {
  id: string
  label: string
  hue: number
  /** Multiplier on the base neutral chroma ramp (0 = pure gray). */
  tint: number
}

export const SURFACE_PRESETS: SurfacePreset[] = [
  // Hue matches the brand primary (#006edc/#007ffe); styles.css bakes this
  // preset's formula in directly so the swatch and applied surface agree.
  { id: 'default', label: 'Default', hue: 256, tint: 2.5 },
  { id: 'graphite', label: 'Graphite', hue: 240, tint: 1 },
  { id: 'neutral', label: 'Neutral', hue: 240, tint: 0 },
  { id: 'slate', label: 'Slate', hue: 255, tint: 3 },
  { id: 'mist', label: 'Mist', hue: 210, tint: 2 },
  { id: 'warm', label: 'Warm', hue: 75, tint: 1.5 },
  { id: 'dusk', label: 'Dusk', hue: 285, tint: 3 },
]

export const DEFAULT_SURFACE = 'default'

/** Neutral lightness/chroma ramps matching the styles.css defaults; chroma is
 *  scaled by the preset tint and re-hued. Foregrounds follow at low chroma. */
function surfaceTokens(preset: SurfacePreset, isDark: boolean): Record<string, string> {
  const { hue: h, tint } = preset
  const c = (base: number) => Math.min(base * tint, 0.045)
  const t = (l: number, base: number) => `oklch(${l} ${c(base)} ${h})`

  if (isDark) {
    return {
      '--background': t(0.165, 0.008),
      '--foreground': t(0.94, 0.006),
      '--card': t(0.205, 0.01),
      '--card-foreground': t(0.94, 0.006),
      '--popover': t(0.205, 0.01),
      '--popover-foreground': t(0.94, 0.006),
      '--secondary': t(0.252, 0.01),
      '--secondary-foreground': t(0.94, 0.006),
      '--muted': t(0.252, 0.01),
      '--muted-foreground': t(0.7, 0.014),
      '--accent': t(0.29, 0.018),
      '--accent-foreground': t(0.955, 0.008),
      '--border': `oklch(0.85 ${c(0.015)} ${h} / 12%)`,
      '--input': `oklch(0.85 ${c(0.015)} ${h} / 15%)`,
      '--sidebar': t(0.14, 0.008),
      '--sidebar-foreground': t(0.94, 0.006),
      '--sidebar-accent': t(0.29, 0.018),
      '--sidebar-accent-foreground': t(0.955, 0.008),
      '--sidebar-border': `oklch(0.85 ${c(0.015)} ${h} / 12%)`,
    }
  }
  return {
    '--background': t(0.99, 0.002),
    '--foreground': t(0.21, 0.012),
    '--card': 'oklch(1 0 0)',
    '--card-foreground': t(0.21, 0.012),
    '--popover': 'oklch(1 0 0)',
    '--popover-foreground': t(0.21, 0.012),
    '--secondary': t(0.966, 0.004),
    '--secondary-foreground': t(0.26, 0.012),
    '--muted': t(0.966, 0.004),
    '--muted-foreground': t(0.545, 0.014),
    '--accent': t(0.955, 0.008),
    '--accent-foreground': t(0.26, 0.016),
    '--border': t(0.916, 0.006),
    '--input': t(0.916, 0.006),
    '--sidebar': t(0.974, 0.004),
    '--sidebar-foreground': t(0.21, 0.012),
    '--sidebar-accent': t(0.948, 0.009),
    '--sidebar-accent-foreground': t(0.26, 0.016),
    '--sidebar-border': t(0.916, 0.006),
  }
}

const SURFACE_VAR_TARGETS = Object.keys(surfaceTokens(SURFACE_PRESETS[0], false))

// ─── Radius / scale ────────────────────────────────────────────────────────────

export const RADIUS_RANGE = { min: 0, max: 1, step: 0.125 } as const
export const DEFAULT_RADIUS = 0.5

export const UI_SCALE_RANGE = { min: 90, max: 115, step: 5 } as const
export const DEFAULT_UI_SCALE = 100

// ─── Persistence ───────────────────────────────────────────────────────────────

const KEYS = {
  accent: 'sqlwarden.preference.accent',
  surface: 'sqlwarden.preference.surface',
  radius: 'sqlwarden.preference.radius',
  uiScale: 'sqlwarden.preference.ui_scale',
} as const

function readAccent(): Accent {
  try {
    const stored = localStorage.getItem(KEYS.accent)
    if (!stored) return DEFAULT_ACCENT
    const parsed = JSON.parse(stored) as Accent
    if (parsed.kind === 'preset' && ACCENT_PRESETS.some((p) => p.id === parsed.id)) return parsed
    if (parsed.kind === 'custom' && /^#[0-9a-f]{6}$/i.test(parsed.hex)) return parsed
  } catch {
    /* fall through */
  }
  return DEFAULT_ACCENT
}

function readSurface(): string {
  const stored = localStorage.getItem(KEYS.surface)
  return stored && SURFACE_PRESETS.some((p) => p.id === stored) ? stored : DEFAULT_SURFACE
}

function readNumber(key: string, fallback: number, min: number, max: number): number {
  const stored = localStorage.getItem(key)
  if (stored) {
    const n = Number(stored)
    if (Number.isFinite(n) && n >= min && n <= max) return n
  }
  return fallback
}

// ─── Provider ──────────────────────────────────────────────────────────────────

type ThemeLabContextValue = {
  accent: Accent
  surface: string
  radius: number
  uiScale: number
  setAccent: (accent: Accent) => void
  setSurface: (surface: string) => void
  setRadius: (radius: number) => void
  setUiScale: (scale: number) => void
  resetThemeLab: () => void
}

const ThemeLabContext = createContext<ThemeLabContextValue>({
  accent: DEFAULT_ACCENT,
  surface: DEFAULT_SURFACE,
  radius: DEFAULT_RADIUS,
  uiScale: DEFAULT_UI_SCALE,
  setAccent: () => {},
  setSurface: () => {},
  setRadius: () => {},
  setUiScale: () => {},
  resetThemeLab: () => {},
})

export function ThemeLabProvider({ children }: { children: ReactNode }) {
  const { resolvedTheme } = useTheme()
  const isDark = resolvedTheme === 'dark'

  const [accent, setAccentState] = useState<Accent>(() => readAccent())
  const [surface, setSurfaceState] = useState<string>(() => readSurface())
  const [radius, setRadiusState] = useState<number>(() =>
    readNumber(KEYS.radius, DEFAULT_RADIUS, RADIUS_RANGE.min, RADIUS_RANGE.max),
  )
  const [uiScale, setUiScaleState] = useState<number>(() =>
    readNumber(KEYS.uiScale, DEFAULT_UI_SCALE, UI_SCALE_RANGE.min, UI_SCALE_RANGE.max),
  )

  // Accent → primary/ring/chart-1 (defaults come from styles.css; only override
  // when the user has moved off the default so stylesheet updates keep winning).
  useEffect(() => {
    const style = document.documentElement.style
    if (accent.kind === 'preset' && accent.id === DEFAULT_ACCENT_ID) {
      for (const v of ACCENT_VAR_TARGETS) style.removeProperty(v)
      style.removeProperty('--primary-foreground')
      style.removeProperty('--sidebar-primary-foreground')
      return
    }
    const { color, foreground } = accentColors(accent, isDark)
    for (const v of ACCENT_VAR_TARGETS) style.setProperty(v, color)
    style.setProperty('--primary-foreground', foreground)
    style.setProperty('--sidebar-primary-foreground', foreground)
  }, [accent, isDark])

  // Surface → neutral ramp.
  useEffect(() => {
    const style = document.documentElement.style
    if (surface === DEFAULT_SURFACE) {
      for (const v of SURFACE_VAR_TARGETS) style.removeProperty(v)
      return
    }
    const preset = SURFACE_PRESETS.find((p) => p.id === surface) ?? SURFACE_PRESETS[0]
    for (const [k, v] of Object.entries(surfaceTokens(preset, isDark))) style.setProperty(k, v)
  }, [surface, isDark])

  // Radius → --radius (all Tailwind radius steps derive from it).
  useEffect(() => {
    const style = document.documentElement.style
    if (radius === DEFAULT_RADIUS) style.removeProperty('--radius')
    else style.setProperty('--radius', `${radius}rem`)
  }, [radius])

  // UI scale → root font-size (rem-based sizing scales with it).
  useEffect(() => {
    const style = document.documentElement.style
    if (uiScale === DEFAULT_UI_SCALE) style.removeProperty('font-size')
    else style.fontSize = `${uiScale}%`
  }, [uiScale])

  function setAccent(next: Accent) {
    localStorage.setItem(KEYS.accent, JSON.stringify(next))
    setAccentState(next)
  }
  function setSurface(next: string) {
    localStorage.setItem(KEYS.surface, next)
    setSurfaceState(next)
  }
  function setRadius(next: number) {
    localStorage.setItem(KEYS.radius, String(next))
    setRadiusState(next)
  }
  function setUiScale(next: number) {
    localStorage.setItem(KEYS.uiScale, String(next))
    setUiScaleState(next)
  }
  function resetThemeLab() {
    for (const key of Object.values(KEYS)) localStorage.removeItem(key)
    setAccentState(DEFAULT_ACCENT)
    setSurfaceState(DEFAULT_SURFACE)
    setRadiusState(DEFAULT_RADIUS)
    setUiScaleState(DEFAULT_UI_SCALE)
  }

  return (
    <ThemeLabContext.Provider
      value={{
        accent,
        surface,
        radius,
        uiScale,
        setAccent,
        setSurface,
        setRadius,
        setUiScale,
        resetThemeLab,
      }}
    >
      {children}
    </ThemeLabContext.Provider>
  )
}

export function useThemeLab() {
  return useContext(ThemeLabContext)
}

/** Swatch color for an accent preset in the current theme (for the picker UI). */
export function accentSwatchColor(preset: AccentPreset, isDark: boolean): string {
  return oklchStr(isDark ? preset.dark : preset.light)
}

/** Representative tone for a surface preset in the current theme (for the picker UI).
 *  Uses the sidebar tone — the most visible "chrome" surface. */
export function surfaceSwatchColor(preset: SurfacePreset, isDark: boolean): string {
  return surfaceTokens(preset, isDark)['--sidebar']
}
