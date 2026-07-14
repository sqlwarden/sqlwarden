import { useState } from 'react'
import { Button } from '#/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '#/components/ui/popover'
import type { Connection, Environment } from '#/lib/api/types'
import { Icon } from '#/lib/icons'
import { cn } from '#/lib/utils'
import { DriverBadge } from './DriverBadge'
import { Tip } from './schema-diagram/Tip'
import { useIde } from './useIdeStore'

export type ConnectionGroup = { environment: Environment; connections: Connection[] }

export function groupConnections(
  environments: Environment[],
  connections: Connection[],
  search: string,
): ConnectionGroup[] {
  const query = search.trim().toLowerCase()
  return environments.flatMap((environment) => {
    const environmentMatches = environment.name.toLowerCase().includes(query)
    const matching = connections.filter(
      (connection) =>
        connection.environment_id === environment.id &&
        (!query || environmentMatches || connection.name.toLowerCase().includes(query)),
    )
    return matching.length > 0 ? [{ environment, connections: matching }] : []
  })
}

export function ConnectionSelector({
  activeConnection,
  activeConnectionId,
  connections,
  environments,
  isLoading,
  tabAvailable,
  onSelect,
}: {
  activeConnection?: Connection
  activeConnectionId?: number
  connections: Connection[]
  environments: Environment[]
  isLoading: boolean
  tabAvailable: boolean
  onSelect: (connection: Connection) => void
}) {
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const sessions = useIde((state) => state.sessions)
  const groups = groupConnections(environments, connections, search)
  const hasConnections = connections.length > 0
  const disabled = !tabAvailable || !hasConnections || isLoading
  const placeholder = isLoading
    ? 'Loading connections…'
    : !hasConnections
      ? 'No connections'
      : 'Select connection…'

  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (!next) setSearch('')
      }}
    >
      <Tip
        label={
          activeConnection
            ? sessions[activeConnection.id]
              ? `Connected to ${activeConnection.name}`
              : 'Not connected — Run connects automatically'
            : 'Choose a connection to run queries'
        }
      >
        <PopoverTrigger
          render={
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={disabled}
              className="h-7 min-w-0 max-w-60 gap-1.5 px-2 text-xs font-normal"
            />
          }
        >
          {activeConnection ? (
            <>
              <DriverBadge driver={activeConnection.driver} size="sm" className="shrink-0" />
              <span className="min-w-0 flex-1 truncate">{activeConnection.name}</span>
              <span
                className={cn(
                  'size-1.5 shrink-0 rounded-full',
                  sessions[activeConnection.id]
                    ? 'bg-green-500'
                    : 'border border-muted-foreground/60',
                )}
              />
            </>
          ) : (
            <>
              <Icon name="database" size={12} className="shrink-0 text-muted-foreground" />
              <span className="text-muted-foreground">{placeholder}</span>
            </>
          )}
          <Icon name="arrow-down-01" size={10} className="ml-0.5 shrink-0 text-muted-foreground" />
        </PopoverTrigger>
      </Tip>

      <PopoverContent align="end" className="w-72 overflow-hidden p-0">
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
                    const active = activeConnectionId === connection.id
                    return (
                      <button
                        key={connection.id}
                        type="button"
                        onClick={() => {
                          onSelect(connection)
                          setOpen(false)
                          setSearch('')
                        }}
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
