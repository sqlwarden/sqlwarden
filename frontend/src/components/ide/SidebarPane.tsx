import type { ComponentType } from 'react'
import { HugeiconsIcon } from '@hugeicons/react'
import { ArrowExpandIcon, ArrowShrinkIcon } from '@hugeicons/core-free-icons'
import { Button } from '#/components/ui/button'
import { ScrollArea } from '#/components/ui/scroll-area'

type SidebarPaneProps = {
  title: string
  icon: ComponentType<{ className?: string }>
  maximized: boolean
  onMaximizedChange: (maximized: boolean) => void
  children: React.ReactNode
}

export function SidebarPane({ title, icon, maximized, onMaximizedChange, children }: SidebarPaneProps) {
  return (
    <section className="flex h-full min-h-0 flex-col">
      <div className="flex h-9 shrink-0 items-center justify-between gap-2 border-b border-border px-2">
        <div className="flex min-w-0 items-center gap-2">
          <HugeiconsIcon icon={icon} size={14} strokeWidth={2} className="shrink-0 text-muted-foreground" />
          <span className="truncate text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
            {title}
          </span>
        </div>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label={`Toggle ${title} maximize`}
          onClick={() => onMaximizedChange(!maximized)}
        >
          <HugeiconsIcon icon={maximized ? ArrowShrinkIcon : ArrowExpandIcon} size={14} strokeWidth={2} />
        </Button>
      </div>
      <ScrollArea className="min-h-0 flex-1">
        <div className="flex flex-col py-1">{children}</div>
      </ScrollArea>
    </section>
  )
}
