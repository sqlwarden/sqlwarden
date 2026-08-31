import { Icon, type AppIcon } from '#/lib/icons'
import { sectionCaptionClass } from '#/lib/typography'
import { cn } from '#/lib/utils'
import { Button } from '#/components/ui/button'
import { ScrollArea } from '#/components/ui/scroll-area'
import { Tip } from './schema-diagram/Tip'

type SidebarPaneProps = {
  title: string
  icon: AppIcon
  maximized?: boolean
  onMaximizedChange?: (maximized: boolean) => void
  actions?: React.ReactNode
  /** Replaces the icon+title block with custom content (e.g. a search input)
   *  for panels whose body is a single control that already states the
   *  panel's purpose, so the caption wouldn't add information. The
   *  maximize/actions slot on the right is unaffected. */
  headerContent?: React.ReactNode
  /** When false, children fill the body without a wrapping ScrollArea
   *  (e.g. the body manages its own scroll/resizable regions). Default true. */
  scroll?: boolean
  children: React.ReactNode
}

export function SidebarPane({
  title,
  icon,
  maximized,
  onMaximizedChange,
  actions,
  headerContent,
  scroll = true,
  children,
}: SidebarPaneProps) {
  return (
    <section className="flex h-full min-h-0 flex-col">
      <div className="flex h-10 shrink-0 items-center justify-between gap-2 border-b border-border px-3">
        {headerContent ? (
          <div className="flex min-w-0 flex-1 items-center gap-2">{headerContent}</div>
        ) : (
          <div className="flex min-w-0 items-center gap-2">
            <Icon name={icon} size={13} className="shrink-0 text-muted-foreground" />
            <span className={cn(sectionCaptionClass, 'truncate')}>{title}</span>
          </div>
        )}
        <div className="flex items-center gap-0.5">
          {actions}
          {onMaximizedChange ? (
            <Tip
              label={
                maximized
                  ? `Restore ${title.toLowerCase()} panel`
                  : `Maximize ${title.toLowerCase()} panel`
              }
            >
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                aria-label={`Toggle ${title} maximize`}
                onClick={() => onMaximizedChange(!maximized)}
              >
                <Icon name={maximized ? 'minimize' : 'maximize'} size={14} />
              </Button>
            </Tip>
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
