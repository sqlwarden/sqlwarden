import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { HugeiconsIcon } from '@hugeicons/react'
import {
  ArrowExpandIcon,
  ArrowShrinkIcon,
  DatabaseIcon,
  PlayIcon,
  ServerStack01Icon,
} from '@hugeicons/core-free-icons'
import { Button } from '#/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '#/components/ui/popover'
import { Separator } from '#/components/ui/separator'
import {
  orgEnvironmentsQueryOptions,
  orgWorkspaceConnectionsQueryOptions,
} from '#/lib/api/query'
import type { Connection, Workspace } from '#/lib/api/types'
import { cn } from '#/lib/utils'
import { useIde } from './useIdeStore'

type IdeToolbarProps = {
  orgSlug: string
  workspace: Workspace
}

export function IdeToolbar({ orgSlug, workspace }: IdeToolbarProps) {
  const [popoverOpen, setPopoverOpen] = useState(false)

  const activeTabId = useIde((s) => s.activeTabId)
  const tabs = useIde((s) => s.tabs)
  const setTabConnection = useIde((s) => s.setTabConnection)
  const maximizedPane = useIde((s) => s.maximizedPane)
  const setMaximizedPane = useIde((s) => s.setMaximizedPane)

  const activeTab = tabs.find((t) => t.id === activeTabId)

  const environments = useQuery(
    orgEnvironmentsQueryOptions(orgSlug, workspace.id, { page_size: 100, sort: 'name', order: 'asc' }),
  )
  const connections = useQuery(
    orgWorkspaceConnectionsQueryOptions(orgSlug, workspace.id, { page_size: 100, sort: 'name', order: 'asc' }),
  )

  const envItems = environments.data?.items ?? []
  const connItems = connections.data?.items ?? []
  const activeConnection = connItems.find((c) => c.id === activeTab?.connectionId)
  const activeEnv = envItems.find((e) => e.id === activeConnection?.environment_id)

  function selectConnection(conn: Connection) {
    if (activeTabId) setTabConnection(activeTabId, conn.id)
    setPopoverOpen(false)
  }

  function toggleMaximize() {
    setMaximizedPane(maximizedPane === 'editor' ? null : 'editor')
  }

  return (
    <div className="flex h-10 shrink-0 items-center gap-2 border-b border-border px-2">
      {/* Connection selector */}
      <Popover open={popoverOpen} onOpenChange={setPopoverOpen}>
        <PopoverTrigger
          render={
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={!activeTab}
              className="h-7 min-w-0 max-w-64 gap-2 text-xs"
            />
          }
        >
          <HugeiconsIcon icon={DatabaseIcon} size={13} strokeWidth={2} className="shrink-0" />
          {activeConnection ? (
            <>
              <span className="truncate font-mono">{activeConnection.name}</span>
              {activeEnv && (
                <span className="shrink-0 text-[10px] text-muted-foreground">· {activeEnv.name}</span>
              )}
            </>
          ) : (
            <span className="text-muted-foreground">Select connection…</span>
          )}
        </PopoverTrigger>
        <PopoverContent align="start" className="w-72 p-1">
          {envItems.length === 0 ? (
            <div className="px-2 py-3 text-xs text-muted-foreground">No connections available.</div>
          ) : (
            envItems.map((env, idx) => {
              const envConns = connItems.filter((c) => c.environment_id === env.id)
              if (!envConns.length) return null
              return (
                <div key={env.id}>
                  {idx > 0 && <Separator className="my-1" />}
                  <div className="flex items-center gap-1.5 px-2 py-1.5">
                    <HugeiconsIcon icon={ServerStack01Icon} size={12} strokeWidth={2} className="text-muted-foreground" />
                    <span className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
                      {env.name}
                    </span>
                  </div>
                  {envConns.map((conn) => (
                    <button
                      key={conn.id}
                      type="button"
                      onClick={() => selectConnection(conn)}
                      className={cn(
                        'flex h-7 w-full items-center gap-2 px-2 text-xs',
                        'transition-colors hover:bg-accent hover:text-accent-foreground',
                        activeTab?.connectionId === conn.id && 'bg-accent text-accent-foreground',
                      )}
                    >
                      <HugeiconsIcon icon={DatabaseIcon} size={13} strokeWidth={2} className="shrink-0 text-muted-foreground" />
                      <span className="min-w-0 flex-1 truncate">{conn.name}</span>
                    </button>
                  ))}
                </div>
              )
            })
          )}
        </PopoverContent>
      </Popover>

      <div className="flex-1" />

      {/* Run button */}
      <Button
        type="button"
        size="sm"
        disabled={!activeConnection}
        onClick={() => {
          // TODO: wire to query execution API
        }}
      >
        <HugeiconsIcon icon={PlayIcon} size={13} strokeWidth={2} data-icon="inline-start" />
        Run
        <kbd className="ml-1 hidden font-mono text-[10px] opacity-60 sm:inline">⌘↵</kbd>
      </Button>

      {/* Maximize toggle */}
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        aria-label="Toggle editor maximize"
        onClick={toggleMaximize}
      >
        <HugeiconsIcon
          icon={maximizedPane === 'editor' ? ArrowShrinkIcon : ArrowExpandIcon}
          size={14}
          strokeWidth={2}
        />
      </Button>
    </div>
  )
}
