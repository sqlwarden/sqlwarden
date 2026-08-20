import { useState } from 'react'
import { Badge } from '#/components/ui/badge'
import { Button } from '#/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '#/components/ui/popover'
import type { Connection, Environment } from '#/lib/api/types'
import { Icon } from '#/lib/icons'
import { cn } from '#/lib/utils'
import { groupConnections } from './ConnectionSelector'
import { DriverBadge } from './DriverBadge'
import { Tip } from './schema-diagram/Tip'
import { useIde } from './useIdeStore'

export const ALL_CONNECTIONS = 'all'

export type HistoryConnectionFilter = number | typeof ALL_CONNECTIONS

export function HistoryConnectionSelector({
  connections,
  environments,
  isLoading,
  value,
  activeHintConnectionId,
  onChange,
}: {
  connections: Connection[]
  environments: Environment[]
  isLoading: boolean
  value: HistoryConnectionFilter
  activeHintConnectionId?: number
  onChange: (value: HistoryConnectionFilter) => void
}) {
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const sessions = useIde((state) => state.sessions)
  const groups = groupConnections(environments, connections, search)
  const hasConnections = connections.length > 0
  const disabled = !hasConnections || isLoading
  const selectedConnection =
    value === ALL_CONNECTIONS ? undefined : connections.find((c) => c.id === value)
  const placeholder = isLoading
    ? 'Loading connections…'
    : !hasConnections
      ? 'No connections'
      : 'All connections'

  function select(next: HistoryConnectionFilter) {
    onChange(next)
    setOpen(false)
    setSearch('')
  }

  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (!next) setSearch('')
      }}
    >
      <PopoverTrigger
        render={
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={disabled}
            aria-label="Filter by connection"
            className="h-7 w-full min-w-0 justify-start gap-1.5 px-2 text-xs font-normal"
          />
        }
      >
        {selectedConnection ? (
          <>
            <DriverBadge driver={selectedConnection.driver} size="sm" className="shrink-0" />
            <span className="min-w-0 flex-1 truncate text-left">{selectedConnection.name}</span>
          </>
        ) : (
          <>
            <Icon name="database" size={12} className="shrink-0 text-muted-foreground" />
            <span className="min-w-0 flex-1 truncate text-left text-muted-foreground">
              {placeholder}
            </span>
          </>
        )}
        <Icon name="arrow-down-01" size={10} className="ml-0.5 shrink-0 text-muted-foreground" />
      </PopoverTrigger>

      <PopoverContent align="start" className="w-72 overflow-hidden p-0">
        {isLoading ? (
          <div className="px-3 py-4 text-center text-xs text-muted-foreground">
            Loading connections…
          </div>
        ) : !hasConnections ? (
          <div className="px-3 py-4 text-center text-xs text-muted-foreground">
            <p className="font-medium text-foreground">No connections</p>
            <p className="mt-0.5">Add a connection to this workspace first.</p>
          </div>
        ) : (
          <>
            <div className="flex items-center gap-2 border-b border-border px-3 py-2">
              <Icon name="search-01" size={12} className="shrink-0 text-muted-foreground" />
              <input
                type="text"
                placeholder="Search connections…"
                value={search}
                onChange={(event) => setSearch(event.target.value)}
                className="min-w-0 flex-1 bg-transparent text-xs outline-none placeholder:text-muted-foreground"
                autoFocus
              />
            </div>
            <div className="border-b border-border/60 py-1">
              <button
                type="button"
                onClick={() => select(ALL_CONNECTIONS)}
                className={cn(
                  'flex h-8 w-full items-center gap-2.5 px-3 text-xs transition-colors',
                  'hover:bg-accent hover:text-accent-foreground',
                  value === ALL_CONNECTIONS && 'bg-accent/60 text-accent-foreground',
                )}
              >
                <Icon name="database" size={12} className="shrink-0 text-muted-foreground" />
                <span className="min-w-0 flex-1 truncate text-left">All connections</span>
                {value === ALL_CONNECTIONS && (
                  <Icon name="checkmark-circle-02" size={13} className="shrink-0 text-primary" />
                )}
              </button>
            </div>
            <div className="max-h-72 overflow-y-auto py-1">
              {groups.map(({ environment, connections: environmentConnections }) => (
                <div key={environment.id} className="mb-1 last:mb-0">
                  <div className="flex items-center gap-1.5 px-3 pb-1 pt-2">
                    <Icon
                      name="server-stack-01"
                      size={11}
                      className="shrink-0 text-muted-foreground/70"
                    />
                    <span className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground/70">
                      {environment.name}
                    </span>
                  </div>
                  {environmentConnections.map((connection) => {
                    const active = value === connection.id
                    return (
                      <button
                        key={connection.id}
                        type="button"
                        onClick={() => select(connection.id)}
                        className={cn(
                          'flex h-8 w-full items-center gap-2.5 px-3 text-xs transition-colors',
                          'hover:bg-accent hover:text-accent-foreground',
                          active && 'bg-accent/60 text-accent-foreground',
                        )}
                      >
                        <DriverBadge driver={connection.driver} size="sm" className="shrink-0" />
                        <span className="min-w-0 flex-1 truncate text-left">{connection.name}</span>
                        {sessions[connection.id] && (
                          <span className="size-1.5 shrink-0 rounded-full bg-green-500" />
                        )}
                        {activeHintConnectionId === connection.id && (
                          <Tip label="Active connection in the current editor">
                            <Badge variant="outline" className="h-4 shrink-0 px-1.5 text-[9px]">
                              Active
                            </Badge>
                          </Tip>
                        )}
                        {active && (
                          <Icon
                            name="checkmark-circle-02"
                            size={13}
                            className="shrink-0 text-primary"
                          />
                        )}
                      </button>
                    )
                  })}
                </div>
              ))}
              {search && groups.length === 0 && (
                <div className="px-3 py-3 text-center text-xs text-muted-foreground">
                  No connections match &quot;{search}&quot;
                </div>
              )}
            </div>
          </>
        )}
      </PopoverContent>
    </Popover>
  )
}
