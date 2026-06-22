import type { ComponentType } from 'react'
import type { AppIcon } from '#/lib/icons'
import type { Workspace } from '#/lib/api/types'
import { DatabasePanel } from './DatabasePanel'
import { FilesPanel } from './FilesPanel'

/** Context every activity surface receives from the IDE shell. */
export type IdeSidebarPanelProps = {
  orgSlug: string
  workspace: Workspace
}

/**
 * An IDE activity. `mode` decides what the activity bar does on click:
 *  - 'sidebar': swap the side panel only (editor/tabs stay).
 *  - 'page':    replace the whole main content region (reserved; unused in v1).
 * `requires` optionally gates visibility; omit to always show.
 */
export type IdeActivity = {
  id: string
  label: string
  icon: AppIcon
  mode: 'sidebar' | 'page'
  component: ComponentType<IdeSidebarPanelProps>
  requires?: () => boolean
}

export const IDE_ACTIVITIES: IdeActivity[] = [
  { id: 'connections', label: 'Explorer', icon: 'database', mode: 'sidebar', component: DatabasePanel },
  { id: 'files', label: 'Files', icon: 'file-01', mode: 'sidebar', component: FilesPanel },
]

/** Activities visible to the current user (honours `requires`). */
export function visibleActivities(): IdeActivity[] {
  return IDE_ACTIVITIES.filter((a) => (a.requires ? a.requires() : true))
}
