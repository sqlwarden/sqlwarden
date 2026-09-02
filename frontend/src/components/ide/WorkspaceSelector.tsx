import { Icon } from '#/lib/icons'
import type { Workspace } from '#/lib/api/types'
import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import {
  Combobox,
  ComboboxEmpty,
  ComboboxIcon,
  ComboboxInput,
  ComboboxInputGroup,
  ComboboxItem,
  ComboboxItemIndicator,
  ComboboxList,
  ComboboxPopup,
  ComboboxTrigger,
  ComboboxValue,
} from '#/components/ui/combobox'
import { Tip } from './schema-diagram/Tip'
import { CreateWorkspaceDialog } from '#/components/workspaces/CreateWorkspaceDialog'

type WorkspaceSelectorProps = {
  workspaces: Workspace[]
  activeWorkspace: Workspace | undefined
  onSelect: (workspaceId: number) => void
  orgSlug?: string
  canCreate?: boolean
  nativeShell?: boolean
  expanded?: boolean
}

/** Replaces the old top-bar workspace tab strip: a searchable switcher at the
 *  bottom of the activity rail, built on Base UI's Combobox so filtering,
 *  selection state, and keyboard navigation come from the primitive rather
 *  than a hand-rolled list. The trigger button is the anchor — it never
 *  moves; only the popup opens and closes next to it. */
export function WorkspaceSelector({
  workspaces,
  activeWorkspace,
  onSelect,
  orgSlug = '',
  canCreate = false,
  nativeShell = false,
  expanded = false,
}: WorkspaceSelectorProps) {
  const [selectorOpen, setSelectorOpen] = useState(false)
  const [createOpen, setCreateOpen] = useState(false)
  const label = activeWorkspace?.name ?? 'Select workspace'

  const trigger = (
    <ComboboxTrigger
      aria-label={label}
      className={expanded ? 'h-8 w-full justify-start gap-2 p-2' : 'size-8 justify-center'}
    >
      <Icon name="briefcase-01" size={17} className="shrink-0" />
      {expanded ? (
        <>
          <span className="min-w-0 flex-1 truncate text-left">
            <ComboboxValue placeholder="Select workspace" />
          </span>
          <ComboboxIcon />
        </>
      ) : null}
    </ComboboxTrigger>
  )

  return (
    <>
      <Combobox
        open={selectorOpen}
        onOpenChange={setSelectorOpen}
        items={workspaces}
        value={activeWorkspace ?? null}
        onValueChange={(workspace: Workspace | null) => {
          if (workspace) onSelect(workspace.id)
        }}
        itemToStringLabel={(workspace: Workspace) => workspace.name}
        isItemEqualToValue={(a: Workspace, b: Workspace) => a.id === b.id}
      >
        {expanded ? (
          trigger
        ) : (
          <Tip label={label} side="right">
            {trigger}
          </Tip>
        )}
        <ComboboxPopup side="right" align="start">
          <ComboboxInputGroup>
            <Icon
              name="search-01"
              size={12}
              className="pointer-events-none absolute top-1/2 start-2 size-3 -translate-y-1/2 text-muted-foreground"
            />
            <ComboboxInput placeholder="Find workspace..." className="ps-7" />
          </ComboboxInputGroup>
          <ComboboxList>
            {(workspace: Workspace) => (
              <ComboboxItem key={workspace.id} value={workspace}>
                <Icon name="briefcase-01" size={14} className="shrink-0 text-muted-foreground" />
                <span className="min-w-0 flex-1 truncate">{workspace.name}</span>
                <ComboboxItemIndicator />
              </ComboboxItem>
            )}
          </ComboboxList>
          <ComboboxEmpty>
            {nativeShell ? 'No workspaces yet.' : 'No workspaces found.'}
          </ComboboxEmpty>
          <div className="flex flex-col gap-0.5 border-t border-border pt-1.5">
            {canCreate ? (
              <button
                type="button"
                className="flex h-8 items-center gap-2 rounded-md px-2 text-left text-xs outline-none hover:bg-sidebar-accent/60 focus-visible:ring-2 focus-visible:ring-ring"
                onClick={() => {
                  setSelectorOpen(false)
                  window.setTimeout(() => setCreateOpen(true), 0)
                }}
              >
                <Icon name="plus-sign" size={14} />
                New workspace
              </button>
            ) : null}
            {nativeShell ? (
              <Link
                to="/desktop/settings"
                className="flex h-8 items-center gap-2 rounded-md px-2 text-xs outline-none hover:bg-sidebar-accent/60 focus-visible:ring-2 focus-visible:ring-ring"
              >
                <Icon name="settings-02" size={14} />
                Manage workspaces
              </Link>
            ) : (
              <Link
                to="/orgs/$org_slug/workspaces"
                params={{ org_slug: orgSlug }}
                className="flex h-8 items-center gap-2 rounded-md px-2 text-xs outline-none hover:bg-sidebar-accent/60 focus-visible:ring-2 focus-visible:ring-ring"
              >
                <Icon name="settings-02" size={14} />
                Manage workspaces
              </Link>
            )}
          </div>
        </ComboboxPopup>
      </Combobox>
      {createOpen ? (
        <CreateWorkspaceDialog
          orgSlug={orgSlug}
          open={createOpen}
          onOpenChange={setCreateOpen}
          onCreated={(workspace) => onSelect(workspace.id)}
        />
      ) : null}
    </>
  )
}
