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
    id: 'blue',
    label: 'Default',
    light: { l: 0.55, c: 0.19, h: 255 },
    dark: { l: 0.68, c: 0.15, h: 250 },
  },
  {
    id: 'steel-teal',
    label: 'Steel Teal',
    light: { l: 0.551, c: 0.188, h: 256 },
    dark: { l: 0.612, c: 0.211, h: 256 },
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

const DEFAULT_ACCENT_ID = 'blue'
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

type DarkSurfaceRamp = {
  background: number
  foreground: number
  card: number
  secondary: number
  mutedForeground: number
  accent: number
  accentForeground: number
  border: number
  sidebar: number
}

type LightSurfaceRamp = {
  background: number
  foreground: number
  secondary: number
  secondaryForeground: number
  mutedForeground: number
  accent: number
  border: number
  sidebar: number
  sidebarAccent: number
}

/** Lightness ramp matching the styles.css defaults. Every preset uses this
 *  unless it defines its own `ramp` override. */
const DEFAULT_DARK_RAMP: DarkSurfaceRamp = {
  background: 0.165,
  foreground: 0.94,
  card: 0.205,
  secondary: 0.252,
  mutedForeground: 0.7,
  accent: 0.29,
  accentForeground: 0.955,
  border: 0.85,
  sidebar: 0.14,
}

const DEFAULT_LIGHT_RAMP: LightSurfaceRamp = {
  background: 0.99,
  foreground: 0.21,
  secondary: 0.966,
  secondaryForeground: 0.26,
  // 0.545 sits right at the 4.5:1 AA floor against `secondary` (muted) with
  // no margin — this preset's chroma tips it just under. 0.52 matches the
  // shipped Default ramp's safety margin (~4.95:1).
  mutedForeground: 0.52,
  accent: 0.955,
  border: 0.916,
  sidebar: 0.974,
  sidebarAccent: 0.948,
}

/** Dark: wide lightness steps between surfaces carry elevation now that
 *  there's no hue/chroma left to do it. Near-black background. */
const MONO_DARK_RAMP: DarkSurfaceRamp = {
  background: 0.085,
  foreground: 0.96,
  card: 0.15,
  secondary: 0.2,
  mutedForeground: 0.62,
  accent: 0.25,
  accentForeground: 0.97,
  border: 0.85,
  sidebar: 0.06,
}

/** Light: the inverse isn't a mirrored high-contrast ramp — a stark near-black
 *  reads as "true black," but the same jump near white reads as flat gray
 *  blocking, not "pure white." Surfaces stay close to background instead,
 *  separated by faint steps and a soft border. */
const MONO_LIGHT_RAMP: LightSurfaceRamp = {
  background: 1,
  foreground: 0.15,
  secondary: 0.99,
  secondaryForeground: 0.2,
  mutedForeground: 0.5,
  accent: 0.985,
  border: 0.96,
  sidebar: 0.995,
  sidebarAccent: 0.98,
}

/** The "Default" ramp — matches the shipped styles.css lightness steps.
 *  Card/popover sit almost flush against the page background (elevation
 *  comes from the border, not a lightness jump), and secondary/muted/accent
 *  share one flat, barely-there wash. This preset uses `tint: 0`, so it
 *  doesn't carry the faint steel-blue chroma styles.css adds to `.dark`
 *  directly — lightness steps match, hue/chroma is a separate axis the
 *  Theme Lab already exposes per-preset. */
const FLUSH_DARK_RAMP: DarkSurfaceRamp = {
  background: 0.19,
  // 0.86, not 0.97 — dimmed to match the shipped styles.css .dark override,
  // which keeps body-text contrast in the ~11-13:1 band soft dark themes
  // (VS Code, Postman) use instead of a harsh ~17:1 near-white-on-near-black.
  foreground: 0.86,
  card: 0.21,
  secondary: 0.23,
  // 0.66, not 0.6 — matches the shipped styles.css .dark override, which
  // lifted this for AA margin against --muted (was 4.28:1, under the 4.5:1
  // floor).
  mutedForeground: 0.66,
  accent: 0.23,
  accentForeground: 0.86,
  border: 0.65,
  sidebar: 0.16,
}

const FLUSH_LIGHT_RAMP: LightSurfaceRamp = {
  background: 1,
  foreground: 0.27,
  secondary: 0.97,
  secondaryForeground: 0.27,
  mutedForeground: 0.52,
  accent: 0.97,
  border: 0.94,
  sidebar: 0.985,
  sidebarAccent: 0.97,
}

export type SurfacePreset = {
  id: string
  label: string
  hue: number
  /** Multiplier on the base neutral chroma ramp (0 = pure gray). */
  tint: number
  /** Overrides the default lightness ramp (background/card/sidebar/... steps)
   *  for presets whose contrast profile diverges from the shared default. */
  ramp?: { dark: DarkSurfaceRamp; light: LightSurfaceRamp }
}

export const SURFACE_PRESETS: SurfacePreset[] = [
  {
    id: 'default',
    label: 'Default',
    hue: 0,
    tint: 0,
    ramp: { dark: FLUSH_DARK_RAMP, light: FLUSH_LIGHT_RAMP },
  },
  { id: 'graphite', label: 'Graphite', hue: 240, tint: 1 },
  { id: 'neutral', label: 'Neutral', hue: 240, tint: 0 },
  { id: 'slate', label: 'Slate', hue: 255, tint: 3 },
  { id: 'mist', label: 'Mist', hue: 210, tint: 2 },
  { id: 'warm', label: 'Warm', hue: 75, tint: 1.5 },
  { id: 'dusk', label: 'Dusk', hue: 285, tint: 3 },
  {
    id: 'mono',
    label: 'Deep Mono',
    hue: 240,
    tint: 0,
    ramp: { dark: MONO_DARK_RAMP, light: MONO_LIGHT_RAMP },
  },
]

export const DEFAULT_SURFACE = 'default'

/** Neutral lightness/chroma ramps matching the styles.css defaults; chroma is
 *  scaled by the preset tint and re-hued. Foregrounds follow at low chroma.
 *  Exported so tests can compute contrast ratios against the real generated
 *  tokens instead of duplicating the ramp constants. */
export function surfaceTokens(preset: SurfacePreset, isDark: boolean): Record<string, string> {
  const { hue: h, tint } = preset
  const c = (base: number) => Math.min(base * tint, 0.045)
  const t = (l: number, base: number) => `oklch(${l} ${c(base)} ${h})`

  if (isDark) {
    const r = preset.ramp?.dark ?? DEFAULT_DARK_RAMP
    return {
      '--background': t(r.background, 0.008),
      '--foreground': t(r.foreground, 0.006),
      '--card': t(r.card, 0.01),
      '--card-foreground': t(r.foreground, 0.006),
      '--popover': t(r.card, 0.01),
      '--popover-foreground': t(r.foreground, 0.006),
      '--secondary': t(r.secondary, 0.01),
      '--secondary-foreground': t(r.foreground, 0.006),
      '--muted': t(r.secondary, 0.01),
      '--muted-foreground': t(r.mutedForeground, 0.014),
      '--accent': t(r.accent, 0.018),
      '--accent-foreground': t(r.accentForeground, 0.008),
      '--border': `oklch(${r.border} ${c(0.015)} ${h} / 12%)`,
      '--input': `oklch(${r.border} ${c(0.015)} ${h} / 15%)`,
      '--sidebar': t(r.sidebar, 0.008),
      '--sidebar-foreground': t(r.foreground, 0.006),
      '--sidebar-accent': t(r.accent, 0.018),
      '--sidebar-accent-foreground': t(r.accentForeground, 0.008),
      '--sidebar-border': `oklch(${r.border} ${c(0.015)} ${h} / 12%)`,
    }
  }
  const r = preset.ramp?.light ?? DEFAULT_LIGHT_RAMP
  return {
    '--background': t(r.background, 0.002),
    '--foreground': t(r.foreground, 0.012),
    '--card': 'oklch(1 0 0)',
    '--card-foreground': t(r.foreground, 0.012),
    '--popover': 'oklch(1 0 0)',
    '--popover-foreground': t(r.foreground, 0.012),
    '--secondary': t(r.secondary, 0.004),
    '--secondary-foreground': t(r.secondaryForeground, 0.012),
    '--muted': t(r.secondary, 0.004),
    '--muted-foreground': t(r.mutedForeground, 0.014),
    '--accent': t(r.accent, 0.008),
    '--accent-foreground': t(r.secondaryForeground, 0.016),
    '--border': t(r.border, 0.006),
    '--input': t(r.border, 0.006),
    '--sidebar': t(r.sidebar, 0.004),
    '--sidebar-foreground': t(r.foreground, 0.012),
    '--sidebar-accent': t(r.sidebarAccent, 0.009),
    '--sidebar-accent-foreground': t(r.secondaryForeground, 0.016),
    '--sidebar-border': t(r.border, 0.006),
  }
}

const SURFACE_VAR_TARGETS = Object.keys(surfaceTokens(SURFACE_PRESETS[0], false))

// ─── Radius / scale ────────────────────────────────────────────────────────────

export const RADIUS_RANGE = { min: 0, max: 1, step: 0.125 } as const
export const DEFAULT_RADIUS = 0.5

export const UI_SCALE_RANGE = { min: 90, max: 115, step: 5 } as const
export const DEFAULT_UI_SCALE = 110

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

  useEffect(() => {
    document.documentElement.style.fontSize = `${uiScale}%`
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
