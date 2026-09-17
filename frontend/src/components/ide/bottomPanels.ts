import type { ComponentType } from 'react'
import type { AppIcon } from '#/lib/icons'
import type { Workspace } from '#/lib/api/types'
import type { ActivityVisibilityContext } from './ideActivities'
import { ResultsArea } from './ResultsArea'
import { HistoryPanel } from './HistoryPanel'
import { FavoritesPanel } from './FavoritesPanel'

/** Context every bottom panel tab receives from the editor shell. */
export type BottomPanelTabProps = {
  orgSlug: string
  workspace: Workspace
  isMaximized: boolean
  onMaximize: () => void
  onClose: () => void
}

/**
 * A tab in the bottom panel (Results / History / Favorites), rendered
 * alongside the editor like a traditional IDE's terminal/debug dock.
 * `requires` optionally gates visibility against org runtime settings;
 * omit to always show.
 */
export type BottomPanelTab = {
  id: string
  label: string
  icon: AppIcon
  component: ComponentType<BottomPanelTabProps>
  requires?: (ctx: ActivityVisibilityContext) => boolean
}

export const BOTTOM_PANELS: BottomPanelTab[] = [
  { id: 'results', label: 'Results', icon: 'table', component: ResultsArea },
  {
    id: 'history',
    label: 'History',
    icon: 'history',
    component: HistoryPanel,
    requires: (ctx) => ctx.queryHistoryMode !== 'off',
  },
  {
    id: 'favorites',
    label: 'Favorites',
    icon: 'star',
    component: FavoritesPanel,
    requires: (ctx) => ctx.queryFavoritesMode !== 'off',
  },
]

/** Bottom panel tabs visible to the current user (honours `requires`). */
export function visibleBottomPanels(ctx: ActivityVisibilityContext): BottomPanelTab[] {
  return BOTTOM_PANELS.filter((p) => (p.requires ? p.requires(ctx) : true))
}
