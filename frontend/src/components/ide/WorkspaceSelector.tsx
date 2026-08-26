import { Icon } from '#/lib/icons'
import type { Workspace } from '#/lib/api/types'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '#/components/ui/dropdown-menu'
import { Tip } from './schema-diagram/Tip'
import { cn } from '#/lib/utils'

type WorkspaceSelectorProps = {
  workspaces: Workspace[]
  activeWorkspace: Workspace | undefined
  onSelect: (workspaceId: number) => void
  expanded?: boolean
}

/** Replaces the old top-bar workspace tab strip: a compact dropdown at the
 *  bottom of the activity rail so the rail stays a fixed width regardless of
 *  how many workspaces an org has. */
export function WorkspaceSelector({
  workspaces,
  activeWorkspace,
  onSelect,
  expanded = false,
}: WorkspaceSelectorProps) {
  const label = activeWorkspace?.name ?? 'Select workspace'
  const trigger = (
    <DropdownMenuTrigger
      aria-label={label}
      className={cn(
        'flex items-center rounded-[calc(var(--radius-sm)+2px)] text-xs text-muted-foreground transition-colors',
        'hover:bg-sidebar-accent/60 hover:text-foreground',
        expanded ? 'h-8 w-full justify-start gap-2 p-2' : 'size-8 justify-center',
      )}
    >
      <Icon name="briefcase-01" size={17} className="shrink-0" />
      {expanded ? <span className="truncate">{label}</span> : null}
    </DropdownMenuTrigger>
  )

  return (
    <DropdownMenu>
      {expanded ? (
        trigger
      ) : (
        <Tip label={label} side="right">
          {trigger}
        </Tip>
      )}
      <DropdownMenuContent align="start" side="right">
        {workspaces.map((workspace) => (
          <DropdownMenuItem key={workspace.id} onClick={() => onSelect(workspace.id)}>
            {workspace.name}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
