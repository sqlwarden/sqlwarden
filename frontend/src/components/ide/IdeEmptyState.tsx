import type { ReactNode } from 'react'
import { Empty, EmptyDescription, EmptyHeader, EmptyTitle } from '#/components/ui/empty'
import { Icon, type AppIcon } from '#/lib/icons'

type IdeEmptyStateProps = {
  icon: AppIcon
  title: string
  description: ReactNode
}

export function IdeEmptyState({ icon, title, description }: IdeEmptyStateProps) {
  return (
    <Empty className="h-full gap-2 rounded-none border-0 p-8">
      <EmptyHeader className="gap-2">
        <EmptyTitle className="flex items-center gap-1.5 text-muted-foreground">
          <Icon name={icon} size={14} className="shrink-0" aria-hidden="true" />
          {title}
        </EmptyTitle>
        <EmptyDescription className="flex items-center justify-center gap-1.5">
          {description}
        </EmptyDescription>
      </EmptyHeader>
    </Empty>
  )
}
