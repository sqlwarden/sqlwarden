import { useTheme } from '#/components/theme-provider'
import { ToggleGroup, ToggleGroupItem } from '#/components/ui/toggle-group'
import { Icon } from '#/lib/icons'
import type { AppIcon } from '#/lib/icons'
import { cn } from '#/lib/utils'

const THEME_OPTIONS: { value: 'light' | 'dark' | 'system'; label: string; icon: AppIcon }[] = [
  { value: 'light', label: 'Light', icon: 'sun' },
  { value: 'dark', label: 'Dark', icon: 'moon' },
  { value: 'system', label: 'System', icon: 'monitor' },
]

export function ThemeSelector({ className }: { className?: string }) {
  const { theme, setTheme } = useTheme()

  return (
    <ToggleGroup
      aria-label="Theme"
      value={[theme]}
      onValueChange={(next) => {
        const selected = next[0]
        if (selected) setTheme(selected as typeof theme)
      }}
      variant="outline"
      size="sm"
      className={cn('rounded-full border-border/70 bg-card p-0.5 shadow-sm', className)}
    >
      {THEME_OPTIONS.map((option) => (
        <ToggleGroupItem
          key={option.value}
          value={option.value}
          aria-label={option.label}
          className="rounded-full border-transparent data-[state=on]:border-transparent data-[state=on]:bg-primary data-[state=on]:text-primary-foreground"
        >
          <Icon name={option.icon} size={13} />
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  )
}
