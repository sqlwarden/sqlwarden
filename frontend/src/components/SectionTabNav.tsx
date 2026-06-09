import { Link } from '@tanstack/react-router'
import { cn } from '#/lib/utils'

export type SectionTab = {
  label: string
  to: string
  params?: Record<string, string>
  isActive: boolean
}

export function SectionTabNav({ tabs }: { tabs: SectionTab[] }) {
  return (
    <div className="flex border-b border-border">
      {tabs.map((tab) => (
        <Link
          key={tab.to}
          to={tab.to as never}
          params={tab.params as never}
          className={cn(
            'relative flex select-none items-center px-4 py-2.5 text-sm font-medium transition-colors',
            tab.isActive
              ? 'text-foreground after:absolute after:inset-x-0 after:bottom-[-1px] after:h-0.5 after:bg-primary'
              : 'text-muted-foreground hover:text-foreground',
          )}
        >
          {tab.label}
        </Link>
      ))}
    </div>
  )
}
