import { Icon, type AppIcon } from '#/lib/icons'
import { Button } from '#/components/ui/button'
import { ScrollArea } from '#/components/ui/scroll-area'

type SidebarPaneProps = {
  title: string
  icon: AppIcon
  maximized?: boolean
  onMaximizedChange?: (maximized: boolean) => void
  actions?: React.ReactNode
  /** When false, children fill the body without a wrapping ScrollArea
   *  (e.g. the body manages its own scroll/resizable regions). Default true. */
  scroll?: boolean
  children: React.ReactNode
}

export function SidebarPane({ title, icon, maximized, onMaximizedChange, actions, scroll = true, children }: SidebarPaneProps) {
  return (
    <section className="flex h-full min-h-0 flex-col">
      <div className="flex h-9 shrink-0 items-center justify-between gap-2 border-b border-border px-2">
        <div className="flex min-w-0 items-center gap-2">
          <Icon name={icon} size={14} className="shrink-0 text-muted-foreground" />
          <span className="truncate text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
            {title}
          </span>
        </div>
        <div className="flex items-center gap-0.5">
          {actions}
          {onMaximizedChange ? (
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label={`Toggle ${title} maximize`}
              onClick={() => onMaximizedChange(!maximized)}
            >
              <Icon name={maximized ? 'minimize' : 'maximize'} size={14} />
            </Button>
          ) : null}
        </div>
      </div>
      {scroll ? (
        <ScrollArea className="min-h-0 flex-1">
          <div className="flex flex-col py-1">{children}</div>
        </ScrollArea>
      ) : (
        <div className="flex min-h-0 flex-1 flex-col">{children}</div>
      )}
    </section>
  )
}
