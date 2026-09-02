import type { ComponentType } from 'react'
import type { AppIcon } from '#/lib/icons'
import type { Workspace } from '#/lib/api/types'
import { DatabasePanel } from './DatabasePanel'
import { FavoritesPanel } from './FavoritesPanel'
import { FilesPanel } from './FilesPanel'
import { HistoryPanel } from './HistoryPanel'
import { ExportsPanel } from './exports/ExportsPanel'
import { SearchPanel } from './SearchPanel'

/** Context every activity surface receives from the editor shell. */
export type IdeSidebarPanelProps = {
  orgSlug: string
  workspace: Workspace
  nativeShell?: boolean
}

/** Org runtime settings relevant to which activities `requires` should show. */
export type ActivityVisibilityContext = {
  queryHistoryMode: string
  queryFavoritesMode: string
}

/**
 * An editor activity. `mode` decides what the activity bar does on click:
 *  - 'sidebar': swap the side panel only (editor/tabs stay).
 *  - 'page':    replace the whole main content region (reserved; unused in v1).
 * `requires` optionally gates visibility against org runtime settings; omit to always show.
 */
export type IdeActivity = {
  id: string
  label: string
  icon: AppIcon
  mode: 'sidebar' | 'page'
  component: ComponentType<IdeSidebarPanelProps>
  requires?: (ctx: ActivityVisibilityContext) => boolean
}

export const IDE_ACTIVITIES: IdeActivity[] = [
  { id: 'search', label: 'Search', icon: 'search-01', mode: 'sidebar', component: SearchPanel },
  {
    id: 'connections',
    label: 'Explorer',
    icon: 'database',
    mode: 'sidebar',
    component: DatabasePanel,
  },
  { id: 'files', label: 'Files', icon: 'file-01', mode: 'sidebar', component: FilesPanel },
  {
    id: 'history',
    label: 'History',
    icon: 'history',
    mode: 'sidebar',
    component: HistoryPanel,
    requires: (ctx) => ctx.queryHistoryMode !== 'off',
  },
  {
    id: 'favorites',
    label: 'Favorites',
    icon: 'star',
    mode: 'sidebar',
    component: FavoritesPanel,
    requires: (ctx) => ctx.queryFavoritesMode !== 'off',
  },
  {
    id: 'exports',
    label: 'Exports',
    icon: 'download-01',
    mode: 'sidebar',
    component: ExportsPanel,
  },
]

/** Activities visible to the current user (honours `requires`). */
export function visibleActivities(ctx: ActivityVisibilityContext): IdeActivity[] {
  return IDE_ACTIVITIES.filter((a) => (a.requires ? a.requires(ctx) : true))
}
