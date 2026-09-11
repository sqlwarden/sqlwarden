import { useMemo, useState } from 'react'
import { Badge } from '#/components/ui/badge'
import { buttonVariants } from '#/components/ui/button'
import {
  Combobox,
  ComboboxCollection,
  ComboboxEmpty,
  ComboboxGroup,
  ComboboxGroupLabel,
  ComboboxIcon,
  ComboboxInput,
  ComboboxInputGroup,
  ComboboxItem,
  ComboboxItemIndicator,
  ComboboxList,
  ComboboxPopup,
  ComboboxTrigger,
} from '#/components/ui/combobox'
import type { Connection, Environment } from '#/lib/api/types'
import { Icon } from '#/lib/icons'
import { cn } from '#/lib/utils'
import { DriverBadge } from './DriverBadge'
import { Tip } from './schema-diagram/Tip'

export const ALL_CONNECTIONS = 'all'

type ConnectionComboboxItem =
  { key: typeof ALL_CONNECTIONS; connection?: undefined } | { key: number; connection: Connection }

interface ConnectionComboboxGroup {
  label: string | null
  items: ConnectionComboboxItem[]
}

function groupConnectionItems(
  environments: Environment[],
  connections: Connection[],
  search: string,
  includeAllOption: boolean,
): ConnectionComboboxGroup[] {
  const query = search.trim().toLowerCase()
  const groups: ConnectionComboboxGroup[] = []
  if (includeAllOption && (!query || 'all connections'.includes(query))) {
    groups.push({ label: null, items: [{ key: ALL_CONNECTIONS }] })
  }
  for (const environment of environments) {
    const environmentMatches = environment.name.toLowerCase().includes(query)
    const matching = connections.filter(
      (connection) =>
        connection.environment_id === environment.id &&
        (!query || environmentMatches || connection.name.toLowerCase().includes(query)),
    )
    if (matching.length > 0) {
      groups.push({
        label: environment.name,
        items: matching.map((connection) => ({ key: connection.id, connection })),
      })
    }
  }
  return groups
}

/** Canonical grouped-by-environment connection picker shared by the IDE toolbar's connection
 *  selector and the query history's connection filter, so both stay visually and behaviorally
 *  in sync with each other and with the rest of the app's searchable dropdowns. */
export function ConnectionCombobox({
  connections,
  environments,
  sessions,
  isLoading,
  disabled,
  value,
  activeHintConnectionId,
  includeAllOption = false,
  placeholder,
  ariaLabel,
  tipLabel,
  align = 'start',
  triggerClassName,
  onSelectConnection,
  onSelectAll,
}: {
  connections: Connection[]
  environments: Environment[]
  sessions: Record<number, unknown>
  isLoading: boolean
  disabled?: boolean
  value?: number | typeof ALL_CONNECTIONS
  /** Rendered next to the connection matching this id, hinting it's active in the current editor tab. */
  activeHintConnectionId?: number
  includeAllOption?: boolean
  placeholder: string
  ariaLabel?: string
  tipLabel?: string
  align?: 'start' | 'end'
  triggerClassName?: string
  onSelectConnection: (connection: Connection) => void
  onSelectAll?: () => void
}) {
  const [search, setSearch] = useState('')
  const hasConnections = connections.length > 0
  const isDisabled = disabled || !hasConnections || isLoading

  const groups = useMemo(
    () => groupConnectionItems(environments, connections, search, includeAllOption),
    [environments, connections, search, includeAllOption],
  )

  const selectedItem: ConnectionComboboxItem | null = useMemo(() => {
    if (value === undefined) return null
    if (value === ALL_CONNECTIONS) return { key: ALL_CONNECTIONS }
    const connection = connections.find((c) => c.id === value)
    return connection ? { key: value, connection } : null
  }, [value, connections])

  function handleValueChange(item: ConnectionComboboxItem | null) {
    if (!item) return
    if (item.key === ALL_CONNECTIONS) {
      onSelectAll?.()
    } else {
      onSelectConnection(item.connection)
    }
    setSearch('')
  }

  const trigger =
    selectedItem?.key === ALL_CONNECTIONS ? (
      <>
        <Icon name="database" size={12} className="shrink-0 text-muted-foreground" />
        <span className="min-w-0 flex-1 truncate text-left">All connections</span>
      </>
    ) : selectedItem?.connection ? (
      <>
        <span className="relative shrink-0">
          <DriverBadge driver={selectedItem.connection.driver} size="sm" />
          <ConnectionSessionDot
            connected={Boolean(sessions[selectedItem.connection.id])}
            ringClassName="ring-background"
          />
        </span>
        <span className="min-w-0 flex-1 truncate text-left">{selectedItem.connection.name}</span>
      </>
    ) : (
      <>
        <Icon name="database" size={12} className="shrink-0 text-muted-foreground" />
        <span className="min-w-0 flex-1 truncate text-left text-muted-foreground">
          {placeholder}
        </span>
      </>
    )

  const comboboxTrigger = (
    <ComboboxTrigger
      aria-label={ariaLabel ?? placeholder}
      disabled={isDisabled}
      className={cn(
        buttonVariants({ variant: 'outline', size: 'sm' }),
        'h-7 min-w-0 max-w-60 justify-start gap-1.5 px-2 text-xs font-normal',
        triggerClassName,
      )}
    >
      {trigger}
      <ComboboxIcon className="ml-0.5" />
    </ComboboxTrigger>
  )

  return (
    <Combobox
      items={groups}
      value={selectedItem}
      onValueChange={handleValueChange}
      onInputValueChange={setSearch}
      itemToStringLabel={(item: ConnectionComboboxItem) =>
        item.key === ALL_CONNECTIONS ? 'All connections' : item.connection.name
      }
      isItemEqualToValue={(a: ConnectionComboboxItem, b: ConnectionComboboxItem) => a.key === b.key}
      filter={null}
      disabled={isDisabled}
    >
      {tipLabel ? <Tip label={tipLabel}>{comboboxTrigger}</Tip> : comboboxTrigger}
      <ComboboxPopup align={align} className="w-72">
        <ComboboxInputGroup>
          <Icon
            name="search-01"
            size={12}
            className="pointer-events-none absolute start-2 top-1/2 size-3 -translate-y-1/2 text-muted-foreground"
          />
          <ComboboxInput placeholder="Search connections…" className="ps-7" />
        </ComboboxInputGroup>
        <ComboboxList className="max-h-72 overflow-y-auto">
          <ComboboxCollection>
            {(group: ConnectionComboboxGroup) => (
              <ComboboxGroup key={group.label ?? ALL_CONNECTIONS} items={group.items}>
                {group.label ? <ComboboxGroupLabel>{group.label}</ComboboxGroupLabel> : null}
                <ComboboxCollection>
                  {(item: ConnectionComboboxItem) => (
                    <ComboboxItem key={item.key} value={item}>
                      {item.key === ALL_CONNECTIONS ? (
                        <>
                          <Icon
                            name="database"
                            size={12}
                            className="shrink-0 text-muted-foreground"
                          />
                          <span className="min-w-0 flex-1 truncate text-left">All connections</span>
                        </>
                      ) : (
                        <>
                          <span className="relative shrink-0">
                            <DriverBadge driver={item.connection.driver} size="sm" />
                            {sessions[item.connection.id] ? (
                              <ConnectionSessionDot ringClassName="ring-popover" />
                            ) : null}
                          </span>
                          <span className="min-w-0 flex-1 truncate text-left">
                            {item.connection.name}
                          </span>
                          {activeHintConnectionId === item.connection.id ? (
                            <Tip label="Active connection in the current editor">
                              <Badge variant="outline" className="h-4 shrink-0 px-1.5 text-[9px]">
                                Active
                              </Badge>
                            </Tip>
                          ) : null}
                        </>
                      )}
                      <ComboboxItemIndicator />
                    </ComboboxItem>
                  )}
                </ComboboxCollection>
              </ComboboxGroup>
            )}
          </ComboboxCollection>
        </ComboboxList>
        <ComboboxEmpty>
          {isLoading
            ? 'Loading connections…'
            : !hasConnections
              ? 'Add a connection to this workspace first.'
              : `No connections match "${search}"`}
        </ComboboxEmpty>
      </ComboboxPopup>
    </Combobox>
  )
}

/** Overlays the driver icon like the schema tree's connection status dot, instead of a
 *  trailing row indicator. `ringClassName` matches the dot's ring to whatever background
 *  it sits on (the toolbar trigger vs. the popup's item rows). */
function ConnectionSessionDot({
  connected = true,
  ringClassName,
}: {
  connected?: boolean
  ringClassName: string
}) {
  return (
    <span
      className={cn(
        'absolute -bottom-0.5 -right-0.5 size-1.5 rounded-full ring-1',
        ringClassName,
        connected ? 'bg-green-500' : 'border border-muted-foreground/60',
      )}
    />
  )
}
