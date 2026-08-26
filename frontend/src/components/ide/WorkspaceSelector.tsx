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
}

/** Replaces the old top-bar workspace tab strip: a compact dropdown at the
 *  bottom of the activity rail so the rail stays a fixed width regardless of
 *  how many workspaces an org has. */
export function WorkspaceSelector({
  workspaces,
  activeWorkspace,
  onSelect,
}: WorkspaceSelectorProps) {
  return (
    <DropdownMenu>
      <Tip label={activeWorkspace?.name ?? 'Select workspace'} side="right">
        <DropdownMenuTrigger
          aria-label={activeWorkspace?.name ?? 'Select workspace'}
          className={cn(
            'flex size-8 items-center justify-center rounded-lg text-muted-foreground transition-colors',
            'hover:bg-sidebar-accent/60 hover:text-foreground',
          )}
        >
          <Icon name="briefcase-01" size={17} />
        </DropdownMenuTrigger>
      </Tip>
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
