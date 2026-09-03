import { useEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { useTheme } from '#/components/theme-provider'
import { Button } from '#/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '#/components/ui/card'
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
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '#/components/ui/collapsible'
import { Icon } from '#/lib/icons'
import {
  useEditorTheme,
  DEFAULT_EDITOR_THEME_DARK,
  DEFAULT_EDITOR_THEME_LIGHT,
} from '#/lib/editor-themes/context'
import { EDITOR_THEME_LABELS, VALID_EDITOR_THEMES } from '#/lib/editor-themes'
import type { EditorThemeName } from '#/lib/editor-themes'
import {
  useEditorFont,
  EDITOR_FONTS,
  EDITOR_FONT_SIZES,
  DEFAULT_EDITOR_FONT,
  DEFAULT_EDITOR_FONT_SIZE,
} from '#/lib/editor-font/context'
import type { EditorFontSize } from '#/lib/editor-font/context'
import {
  useHeadingFont,
  loadHeadingFont,
  HEADING_FONTS,
  DEFAULT_HEADING_FONT,
} from '#/lib/heading-font/context'
import {
  useInterfaceFont,
  loadInterfaceFont,
  INTERFACE_FONTS,
  DEFAULT_INTERFACE_FONT,
} from '#/lib/interface-font/context'
import {
  useThemeLab,
  accentSwatchColor,
  ACCENT_PRESETS,
  SURFACE_PRESETS,
  RADIUS_RANGE,
  UI_SCALE_RANGE,
} from '#/lib/theme-lab/context'
import type { Accent } from '#/lib/theme-lab/context'
import { useAppShellPreferences } from '#/components/app-shell-preferences'
import type { AppShellSidebarStyle } from '#/components/app-shell-preferences'
import { cn } from '#/lib/utils'
import { EditorThemePreview } from './EditorThemePreview'

/** Appearance controls laid out as label/control rows to match the settings
 *  pages. `isDev` is a prop only so tests can force both states; it defaults to
 *  the build-time dev flag. */
export function AppearancePanel(props: { isDev?: boolean }) {
  const isDev = props.isDev ?? import.meta.env.DEV

  const { theme, resolvedTheme, setTheme } = useTheme()
  const isDark = resolvedTheme === 'dark'

  const { editorThemeDark, editorThemeLight, setEditorThemeDark, setEditorThemeLight } =
    useEditorTheme()
  const { editorFont, editorFontSize, setEditorFont, setEditorFontSize } = useEditorFont()
  const { headingFont, setHeadingFont } = useHeadingFont()
  const { interfaceFont, setInterfaceFont } = useInterfaceFont()
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
  const { preferences, setPreferences } = useAppShellPreferences()

  function resetAppearance() {
    setTheme('system')
    setSurface('default')
    setUiScale(100)
    setEditorThemeDark(DEFAULT_EDITOR_THEME_DARK)
    setEditorThemeLight(DEFAULT_EDITOR_THEME_LIGHT)
    setEditorFont(DEFAULT_EDITOR_FONT)
    setEditorFontSize(DEFAULT_EDITOR_FONT_SIZE)
  }

  function resetDeveloperOptions() {
    resetThemeLab()
    setHeadingFont(DEFAULT_HEADING_FONT)
    setInterfaceFont(DEFAULT_INTERFACE_FONT)
    setPreferences((current) => ({ ...current, sidebarStyle: 'sidebar' }))
  }

  const fontSizeMin = EDITOR_FONT_SIZES[0]
  const fontSizeMax = EDITOR_FONT_SIZES[EDITOR_FONT_SIZES.length - 1]

  return (
    <div className="flex flex-col gap-4">
      <Card>
        <CardHeader className="border-b border-border">
          <CardTitle>General</CardTitle>
        </CardHeader>
        <CardContent className="divide-y divide-border">
          <Row title="Theme">
            <ModeToggle
              value={theme}
              options={['light', 'dark', 'system']}
              onValueChange={(next) => setTheme(next as typeof theme)}
            />
          </Row>

          <Row title="Surface">
            <ModeToggle
              value={surface}
              options={SURFACE_PRESETS.map((preset) => preset.id)}
              labels={Object.fromEntries(
                SURFACE_PRESETS.map((preset) => [preset.id, preset.label]),
              )}
              onValueChange={setSurface}
            />
          </Row>

          <Row title="UI Scale">
            <SliderControl
              value={uiScale}
              min={UI_SCALE_RANGE.min}
              max={UI_SCALE_RANGE.max}
              step={UI_SCALE_RANGE.step}
              display={`${uiScale}%`}
              onValueChange={setUiScale}
            />
          </Row>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="border-b border-border">
          <CardTitle>Editor</CardTitle>
        </CardHeader>
        <CardContent className="divide-y divide-border">
          <Row title="Dark Theme">
            <ThemeSelect value={editorThemeDark} onValueChange={setEditorThemeDark} />
          </Row>
          <Row title="Light Theme">
            <ThemeSelect value={editorThemeLight} onValueChange={setEditorThemeLight} />
          </Row>
          <Row title="Font">
            <FontSelect
              value={editorFont.fontFamily}
              fonts={EDITOR_FONTS}
              onValueChange={setEditorFont}
            />
          </Row>
          <Row title="Font Size">
            <SliderControl
              value={editorFontSize}
              min={fontSizeMin}
              max={fontSizeMax}
              step={1}
              display={`${editorFontSize}px`}
              onValueChange={(v) => setEditorFontSize(v as EditorFontSize)}
            />
          </Row>
          <div className="pt-4">
            <EditorThemePreview />
          </div>
        </CardContent>
      </Card>

      {isDev ? (
        <Collapsible defaultOpen={false}>
          <CollapsibleTrigger className="group flex w-full items-center gap-2 py-1 text-xs font-medium text-foreground">
            <Icon
              name="chevron-right"
              size={16}
              className="text-muted-foreground transition-transform group-data-[panel-open]:rotate-90"
            />
            Developer options
            <span className="rounded-full border border-border px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
              Dev only
            </span>
          </CollapsibleTrigger>
          <CollapsibleContent>
            <Card className="mt-2 border border-dashed border-border">
              <CardContent className="divide-y divide-border">
                <Row title="Accent">
                  <AccentPicker value={accent} isDark={isDark} onValueChange={setAccent} />
                </Row>
                <Row title="Border Radius">
                  <SliderControl
                    value={radius}
                    min={RADIUS_RANGE.min}
                    max={RADIUS_RANGE.max}
                    step={RADIUS_RANGE.step}
                    display={`${radius}rem`}
                    onValueChange={setRadius}
                  />
                </Row>
                <Row title="Heading Font">
                  <FontSelect
                    value={headingFont.fontFamily}
                    fonts={HEADING_FONTS}
                    onValueChange={setHeadingFont}
                    preload={loadHeadingFont}
                  />
                </Row>
                <Row title="Interface Font">
                  <FontSelect
                    value={interfaceFont.fontFamily}
                    fonts={INTERFACE_FONTS}
                    onValueChange={setInterfaceFont}
                    preload={loadInterfaceFont}
                  />
                </Row>
                <Row title="Sidebar Style">
                  <ModeToggle
                    value={preferences.sidebarStyle}
                    options={['inset', 'sidebar', 'floating']}
                    labels={{ inset: 'Inset', sidebar: 'Sidebar', floating: 'Floating' }}
                    onValueChange={(value) =>
                      setPreferences((current) => ({
                        ...current,
                        sidebarStyle: value as AppShellSidebarStyle,
                      }))
                    }
                  />
                </Row>
                <div className="py-2">
                  <Button type="button" variant="outline" size="sm" onClick={resetDeveloperOptions}>
                    Reset developer options
                  </Button>
                </div>
              </CardContent>
            </Card>
          </CollapsibleContent>
        </Collapsible>
      ) : null}

      <div className="flex justify-end border-t border-border pt-3">
        <Button type="button" variant="ghost" size="sm" onClick={resetAppearance}>
          Reset to defaults
        </Button>
      </div>
    </div>
  )
}

function Row({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="flex items-center gap-4 py-2.5">
      <span className="shrink-0 text-xs font-medium text-foreground">{title}</span>
      <div className="flex min-w-0 flex-1 items-center justify-end">{children}</div>
    </div>
  )
}

function ModeToggle({
  value,
  options,
  labels,
  onValueChange,
}: {
  value: string
  options: string[]
  labels?: Record<string, string>
  onValueChange: (value: string) => void
}) {
  return (
    <ToggleGroup
      size="sm"
      variant="outline"
      value={[value]}
      onValueChange={(next) => {
        const selected = next[0]
        if (selected) onValueChange(selected)
      }}
      className="**:data-[slot=toggle-group-item]:px-3 **:data-[slot=toggle-group-item]:text-xs"
    >
      {options.map((option) => {
        const label = labels?.[option] ?? titleCase(option)
        return (
          <ToggleGroupItem key={option} value={option} aria-label={label}>
            {label}
          </ToggleGroupItem>
        )
      })}
    </ToggleGroup>
  )
}

function SliderControl({
  value,
  min,
  max,
  step,
  display,
  onValueChange,
}: {
  value: number
  min: number
  max: number
  step: number
  display: string
  onValueChange: (value: number) => void
}) {
  return (
    <div className="flex w-full items-center gap-3">
      <Slider
        value={[value]}
        min={min}
        max={max}
        step={step}
        className="flex-1"
        onValueChange={(val) => {
          const v = Array.isArray(val) ? val[0] : val
          if (typeof v === 'number') onValueChange(v)
        }}
      />
      <span className="w-12 shrink-0 text-right text-xs tabular-nums text-muted-foreground">
        {display}
      </span>
    </div>
  )
}

function ThemeSelect({
  value,
  onValueChange,
}: {
  value: EditorThemeName
  onValueChange: (value: EditorThemeName) => void
}) {
  const items = VALID_EDITOR_THEMES.map((name) => ({
    label: EDITOR_THEME_LABELS[name],
    value: name,
  }))

  return (
    <Select
      items={items}
      value={value}
      onValueChange={(v) => v && onValueChange(v as EditorThemeName)}
    >
      <SelectTrigger size="sm" className="w-52">
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
  )
}

type NamedFont = { fontFamily: string; label: string }

function FontSelect<T extends NamedFont>({
  value,
  fonts,
  onValueChange,
  preload,
}: {
  value: string
  fonts: readonly T[]
  onValueChange: (font: T) => void
  preload?: (font: T) => Promise<unknown>
}) {
  // Preload every option once the picker mounts so each previews in its own face.
  useEffect(() => {
    if (!preload) return
    for (const font of fonts) void preload(font)
  }, [fonts, preload])

  const items = fonts.map((f) => ({ label: f.label, value: f.fontFamily }))

  return (
    <Select
      items={items}
      value={value}
      onValueChange={(v) => {
        if (!v) return
        const found = fonts.find((f) => f.fontFamily === v)
        if (found) onValueChange(found)
      }}
    >
      <SelectTrigger size="sm" className="w-52">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectGroup>
          {fonts.map((f) => (
            <SelectItem key={f.fontFamily} value={f.fontFamily}>
              <span style={{ fontFamily: f.fontFamily }}>{f.label}</span>
            </SelectItem>
          ))}
        </SelectGroup>
      </SelectContent>
    </Select>
  )
}

function AccentPicker({
  value,
  isDark,
  onValueChange,
}: {
  value: Accent
  isDark: boolean
  onValueChange: (accent: Accent) => void
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
  )
}

function titleCase(value: string) {
  return value
    .split('-')
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}
