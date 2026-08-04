import { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import type { Dispatch, SetStateAction } from 'react'
import { Icon, useIconPack, DEFAULT_ICON_PACK } from '#/lib/icons'
import type { IconPackName } from '#/lib/icons'
import { useTheme } from '#/components/theme-provider'
import { Button } from '#/components/ui/button'
import { ToggleGroup, ToggleGroupItem } from '#/components/ui/toggle-group'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '#/components/ui/select'
import { Slider } from '#/components/ui/slider'
import {
  useEditorTheme,
  DEFAULT_EDITOR_THEME_DARK,
  DEFAULT_EDITOR_THEME_LIGHT,
} from '#/lib/editor-themes/context'
import { EDITOR_THEME_LABELS, VALID_EDITOR_THEMES } from '#/lib/editor-themes'
import type { EditorThemeName } from '#/lib/editor-themes'
import { useConnectionLayout } from '#/components/ide/useConnectionLayout'
import type { ConnectionLayout } from '#/components/ide/useConnectionLayout'
import {
  useEditorFont,
  EDITOR_FONTS,
  EDITOR_FONT_SIZES,
  DEFAULT_EDITOR_FONT,
  DEFAULT_EDITOR_FONT_SIZE,
} from '#/lib/editor-font/context'
import type { EditorFont, EditorFontSize } from '#/lib/editor-font/context'
import {
  useInterfaceFont,
  loadInterfaceFont,
  INTERFACE_FONTS,
  DEFAULT_INTERFACE_FONT,
} from '#/lib/interface-font/context'
import type { InterfaceFont } from '#/lib/interface-font/context'
import {
  useThemeLab,
  accentSwatchColor,
  surfaceSwatchColor,
  ACCENT_PRESETS,
  SURFACE_PRESETS,
  RADIUS_RANGE,
  UI_SCALE_RANGE,
} from '#/lib/theme-lab/context'
import { Tip } from '#/components/ide/schema-diagram/Tip'
import { cn } from '#/lib/utils'
import { appShellPreferenceKeys, defaultAppShellPreferences } from './app-shell-preferences'
import type {
  AppShellPreferences,
  AppShellSidebarStyle,
  AppShellTheme,
} from './app-shell-preferences'

/** Floating, draggable, non-modal panel: the app stays fully interactive while
 *  it is open, so every tweak is visible live in the surface being styled. */
export function UiLabPanel({
  preferences,
  setPreferences,
  onClose,
}: {
  preferences: AppShellPreferences
  setPreferences: Dispatch<SetStateAction<AppShellPreferences>>
  onClose: () => void
}) {
  const { packName, setPackName } = useIconPack()
  const { editorThemeDark, editorThemeLight, setEditorThemeDark, setEditorThemeLight } =
    useEditorTheme()
  const { editorFont, editorFontSize, setEditorFont, setEditorFontSize } = useEditorFont()
  const { interfaceFont, setInterfaceFont } = useInterfaceFont()
  const { connectionLayout, setConnectionLayout } = useConnectionLayout()
  const {
    accent,
    surface,
    radius,
    uiScale,
    setAccent,
    setSurface,
    setRadius,
    setUiScale,
    resetThemeLab,
  } = useThemeLab()
  const { resolvedTheme } = useTheme()
  const isDark = resolvedTheme === 'dark'

  const panelRef = useRef<HTMLDivElement>(null)
  const [pos, setPos] = useState<{ x: number; y: number } | null>(null)

  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [onClose])

  function handleDragStart(e: React.PointerEvent) {
    const panel = panelRef.current
    if (!panel) return
    const rect = panel.getBoundingClientRect()
    const offsetX = e.clientX - rect.left
    const offsetY = e.clientY - rect.top

    function onMove(ev: PointerEvent) {
      const width = panel?.offsetWidth ?? 320
      const height = panel?.offsetHeight ?? 200
      setPos({
        x: Math.min(Math.max(ev.clientX - offsetX, 8), window.innerWidth - width - 8),
        y: Math.min(Math.max(ev.clientY - offsetY, 8), window.innerHeight - Math.min(height, 120)),
      })
    }
    function onUp() {
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
    }
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
  }

  function updatePreference<Key extends keyof AppShellPreferences>(
    key: Key,
    value: AppShellPreferences[Key],
  ) {
    window.localStorage.setItem(appShellPreferenceKeys[key], value)
    setPreferences((current) => ({
      ...current,
      [key]: value,
    }))
  }

  function restoreDefaults() {
    Object.entries(appShellPreferenceKeys).forEach(([key, storageKey]) => {
      const typedKey = key as keyof AppShellPreferences
      window.localStorage.setItem(storageKey, defaultAppShellPreferences[typedKey])
    })
    setPreferences(defaultAppShellPreferences)
    setPackName(DEFAULT_ICON_PACK)
    setEditorThemeDark(DEFAULT_EDITOR_THEME_DARK)
    setEditorThemeLight(DEFAULT_EDITOR_THEME_LIGHT)
    setEditorFont(DEFAULT_EDITOR_FONT)
    setEditorFontSize(DEFAULT_EDITOR_FONT_SIZE)
    setInterfaceFont(DEFAULT_INTERFACE_FONT)
    resetThemeLab()
  }

  return createPortal(
    <div
      ref={panelRef}
      role="dialog"
      aria-label="UI Lab"
      style={pos ? { left: pos.x, top: pos.y, right: 'auto', bottom: 'auto' } : undefined}
      className="fixed bottom-4 right-4 z-50 flex max-h-[calc(100dvh-2rem)] w-80 flex-col overflow-hidden rounded-xl border border-border bg-popover text-popover-foreground shadow-2xl"
    >
      <div
        onPointerDown={handleDragStart}
        className="flex shrink-0 cursor-grab select-none items-center gap-2 border-b border-border px-3 py-2.5 active:cursor-grabbing"
      >
        <Icon name="paint-board" size={14} className="shrink-0 text-primary" />
        <div className="min-w-0 flex-1">
          <div className="text-xs font-semibold leading-none">UI Lab</div>
          <div className="mt-0.5 truncate text-[10px] text-muted-foreground">
            Live controls — drag me anywhere, the app stays interactive.
          </div>
        </div>
        <Tip label="Reset all UI preferences">
          <Button type="button" variant="ghost" size="sm" onClick={restoreDefaults}>
            Reset
          </Button>
        </Tip>
        <Tip label="Close UI Lab">
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label="Close UI Lab"
            onClick={onClose}
          >
            <Icon name="cancel-01" size={13} />
          </Button>
        </Tip>
      </div>

      <div className="flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto p-3">
        <div className="flex flex-col gap-3 **:data-[slot=toggle-group]:w-full **:data-[slot=toggle-group-item]:flex-1 **:data-[slot=toggle-group-item]:text-xs">
          <div className="text-xs font-medium text-muted-foreground">Theme</div>
          <PreferenceToggle
            label="Theme Mode"
            value={preferences.themeMode}
            options={['light', 'dark', 'system']}
            onValueChange={(value) => updatePreference('themeMode', value as AppShellTheme)}
          />

          <AccentPicker value={accent} isDark={isDark} onValueChange={setAccent} />
          <SurfacePicker value={surface} isDark={isDark} onValueChange={setSurface} />

          <LabSlider
            label="Border Radius"
            display={`${radius}rem`}
            value={radius}
            min={RADIUS_RANGE.min}
            max={RADIUS_RANGE.max}
            step={RADIUS_RANGE.step}
            onValueChange={setRadius}
          />
          <LabSlider
            label="UI Scale"
            display={`${uiScale}%`}
            value={uiScale}
            min={UI_SCALE_RANGE.min}
            max={UI_SCALE_RANGE.max}
            step={UI_SCALE_RANGE.step}
            onValueChange={setUiScale}
          />
        </div>

        <div className="flex flex-col gap-3 **:data-[slot=toggle-group]:w-full **:data-[slot=toggle-group-item]:flex-1 **:data-[slot=toggle-group-item]:text-xs">
          <div className="text-xs font-medium text-muted-foreground">App</div>
          <InterfaceFontSelect value={interfaceFont} onValueChange={setInterfaceFont} />

          <PreferenceToggle
            label="Sidebar Style"
            value={preferences.sidebarStyle}
            options={['inset', 'sidebar', 'floating']}
            labels={{ inset: 'Inset', sidebar: 'Sidebar', floating: 'Floating' }}
            onValueChange={(value) =>
              updatePreference('sidebarStyle', value as AppShellSidebarStyle)
            }
          />

          <PreferenceToggle
            label="Icon Pack"
            value={packName}
            options={['hugeicons', 'lucide', 'remix']}
            labels={{ hugeicons: 'HugeIcons', lucide: 'Lucide', remix: 'Remix' }}
            onValueChange={(value) => setPackName(value as IconPackName)}
          />

          <PreferenceToggle
            label="Connections"
            value={connectionLayout}
            options={['flat', 'grouped']}
            labels={{ flat: 'Flat', grouped: 'By Environment' }}
            onValueChange={(value) => setConnectionLayout(value as ConnectionLayout)}
          />
        </div>

        <div className="flex flex-col gap-3">
          <div className="text-xs font-medium text-muted-foreground">Editor</div>
          <EditorThemeSelect
            label="Dark Theme"
            value={editorThemeDark}
            onValueChange={setEditorThemeDark}
          />
          <EditorThemeSelect
            label="Light Theme"
            value={editorThemeLight}
            onValueChange={setEditorThemeLight}
          />
          <EditorFontSelect value={editorFont} onValueChange={setEditorFont} />
          <EditorFontSizeSlider value={editorFontSize} onValueChange={setEditorFontSize} />
        </div>
      </div>
    </div>,
    document.body,
  )
}

function PreferenceToggle({
  label,
  value,
  options,
  labels,
  onValueChange,
}: {
  label: string
  value: string
  options: string[]
  labels?: Record<string, string>
  onValueChange: (value: string) => void
}) {
  return (
    <div className="flex flex-col gap-1">
      <div className="text-xs font-medium">{label}</div>
      <ToggleGroup
        size="sm"
        variant="outline"
        value={[value]}
        onValueChange={(nextValue) => {
          const selected = nextValue[0]
          if (selected) onValueChange(selected)
        }}
      >
        {options.map((option) => (
          <ToggleGroupItem key={option} value={option} aria-label={labels?.[option] ?? option}>
            {labels?.[option] ?? titleCase(option)}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
    </div>
  )
}

function EditorThemeSelect({
  label,
  value,
  onValueChange,
}: {
  label: string
  value: EditorThemeName
  onValueChange: (value: EditorThemeName) => void
}) {
  const items = VALID_EDITOR_THEMES.map((name) => ({
    label: EDITOR_THEME_LABELS[name],
    value: name,
  }))

  return (
    <div className="flex flex-col gap-1">
      <div className="text-xs font-medium">{label}</div>
      <Select
        items={items}
        value={value}
        onValueChange={(v) => v && onValueChange(v as EditorThemeName)}
      >
        <SelectTrigger size="sm" className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            {VALID_EDITOR_THEMES.map((name) => (
              <SelectItem key={name} value={name}>
                {EDITOR_THEME_LABELS[name]}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </div>
  )
}

function AccentPicker({
  value,
  isDark,
  onValueChange,
}: {
  value: import('#/lib/theme-lab/context').Accent
  isDark: boolean
  onValueChange: (accent: import('#/lib/theme-lab/context').Accent) => void
}) {
  const customActive = value.kind === 'custom'

  // The native color input fires per mousemove while dragging; applying each
  // tick would restyle the whole page (root CSS vars) and tank the frame rate.
  // Local state keeps the swatch live while the theme commit trails behind.
  const [draftHex, setDraftHex] = useState<string | null>(null)
  const commitTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  useEffect(
    () => () => {
      if (commitTimerRef.current) clearTimeout(commitTimerRef.current)
    },
    [],
  )

  function handleCustomChange(hex: string) {
    setDraftHex(hex)
    if (commitTimerRef.current) clearTimeout(commitTimerRef.current)
    commitTimerRef.current = setTimeout(() => onValueChange({ kind: 'custom', hex }), 90)
  }

  const customHex = draftHex ?? (customActive ? value.hex : '#0e7490')

  return (
    <div className="flex flex-col gap-1.5">
      <div className="text-xs font-medium">Accent</div>
      <div className="flex flex-wrap items-center gap-1.5">
        {ACCENT_PRESETS.map((preset) => {
          const active = value.kind === 'preset' && value.id === preset.id
          return (
            <button
              key={preset.id}
              type="button"
              title={preset.label}
              aria-label={`Accent: ${preset.label}`}
              aria-pressed={active}
              onClick={() => {
                setDraftHex(null)
                onValueChange({ kind: 'preset', id: preset.id })
              }}
              style={{ backgroundColor: accentSwatchColor(preset, isDark) }}
              className={cn(
                'size-6 shrink-0 rounded-full border border-black/10 transition-transform hover:scale-110 dark:border-white/15',
                active && 'ring-2 ring-ring ring-offset-2 ring-offset-popover',
              )}
            />
          )
        })}
        <label
          title="Custom color"
          className={cn(
            'relative size-6 shrink-0 cursor-pointer overflow-hidden rounded-full border border-black/10 transition-transform hover:scale-110 dark:border-white/15',
            customActive && 'ring-2 ring-ring ring-offset-2 ring-offset-popover',
          )}
          style={{
            background:
              customActive || draftHex
                ? customHex
                : 'conic-gradient(oklch(0.7 0.18 0), oklch(0.8 0.16 90), oklch(0.7 0.16 180), oklch(0.6 0.2 270), oklch(0.7 0.18 360))',
          }}
        >
          <input
            type="color"
            value={customHex}
            aria-label="Custom accent color"
            onChange={(e) => handleCustomChange(e.target.value)}
            className="absolute inset-0 size-full cursor-pointer opacity-0"
          />
        </label>
      </div>
    </div>
  )
}

function SurfacePicker({
  value,
  isDark,
  onValueChange,
}: {
  value: string
  isDark: boolean
  onValueChange: (surface: string) => void
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <div className="text-xs font-medium">Surface</div>
      <div className="grid grid-cols-3 gap-1.5">
        {SURFACE_PRESETS.map((preset) => {
          const active = value === preset.id
          return (
            <button
              key={preset.id}
              type="button"
              aria-pressed={active}
              onClick={() => onValueChange(preset.id)}
              className={cn(
                'flex flex-col items-center gap-1 rounded-md border p-1.5 text-[10px] transition-colors',
                active
                  ? 'border-ring bg-accent/50 text-foreground'
                  : 'border-border text-muted-foreground hover:border-ring/40 hover:text-foreground',
              )}
            >
              <span
                className="h-3.5 w-full rounded-sm border border-black/10 dark:border-white/15"
                style={{ backgroundColor: surfaceSwatchColor(preset, isDark) }}
              />
              {preset.label}
            </button>
          )
        })}
      </div>
    </div>
  )
}

function LabSlider({
  label,
  display,
  value,
  min,
  max,
  step,
  onValueChange,
}: {
  label: string
  display: string
  value: number
  min: number
  max: number
  step: number
  onValueChange: (value: number) => void
}) {
  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center justify-between">
        <div className="text-xs font-medium">{label}</div>
        <div className="text-xs tabular-nums text-muted-foreground">{display}</div>
      </div>
      <Slider
        value={[value]}
        min={min}
        max={max}
        step={step}
        onValueChange={(val) => {
          const v = Array.isArray(val) ? val[0] : val
          if (typeof v === 'number') onValueChange(v)
        }}
      />
    </div>
  )
}

function InterfaceFontSelect({
  value,
  onValueChange,
}: {
  value: InterfaceFont
  onValueChange: (font: InterfaceFont) => void
}) {
  // Preload every interface font once the picker is on screen so each option
  // previews in its own face while the user experiments.
  useEffect(() => {
    for (const font of INTERFACE_FONTS) void loadInterfaceFont(font)
  }, [])

  const items = INTERFACE_FONTS.map((f) => ({ label: f.label, value: f.fontFamily }))

  return (
    <div className="flex flex-col gap-1">
      <div className="text-xs font-medium">Interface Font</div>
      <Select
        items={items}
        value={value.fontFamily}
        onValueChange={(v) => {
          if (!v) return
          const found = INTERFACE_FONTS.find((f) => f.fontFamily === v)
          if (found) onValueChange(found)
        }}
      >
        <SelectTrigger size="sm" className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            {INTERFACE_FONTS.map((f) => (
              <SelectItem key={f.fontFamily} value={f.fontFamily}>
                <span style={{ fontFamily: f.fontFamily }}>{f.label}</span>
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </div>
  )
}

function EditorFontSelect({
  value,
  onValueChange,
}: {
  value: EditorFont
  onValueChange: (font: EditorFont) => void
}) {
  const items = EDITOR_FONTS.map((f) => ({ label: f.label, value: f.fontFamily }))

  return (
    <div className="flex flex-col gap-1">
      <div className="text-xs font-medium">Font</div>
      <Select
        items={items}
        value={value.fontFamily}
        onValueChange={(v) => {
          if (!v) return
          const found = EDITOR_FONTS.find((f) => f.fontFamily === v)
          if (found) onValueChange(found)
        }}
      >
        <SelectTrigger size="sm" className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            {EDITOR_FONTS.map((f) => (
              <SelectItem key={f.fontFamily} value={f.fontFamily}>
                {f.label}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </div>
  )
}

function EditorFontSizeSlider({
  value,
  onValueChange,
}: {
  value: EditorFontSize
  onValueChange: (size: EditorFontSize) => void
}) {
  const min = EDITOR_FONT_SIZES[0]
  const max = EDITOR_FONT_SIZES[EDITOR_FONT_SIZES.length - 1]

  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center justify-between">
        <div className="text-xs font-medium">Font Size</div>
        <div className="text-xs tabular-nums text-muted-foreground">{value}px</div>
      </div>
      <Slider
        value={[value]}
        min={min}
        max={max}
        step={1}
        onValueChange={(val) => {
          const v = Array.isArray(val) ? val[0] : val
          if (typeof v === 'number') onValueChange(v as EditorFontSize)
        }}
      />
    </div>
  )
}

function titleCase(value: string) {
  return value
    .split('-')
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}
