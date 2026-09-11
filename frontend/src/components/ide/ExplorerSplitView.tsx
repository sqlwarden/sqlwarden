import { useState, type RefObject } from 'react'
import type { PanelImperativeHandle } from 'react-resizable-panels'
import { SearchInput } from '#/components/SearchInput'
import { Button } from '#/components/ui/button'
import { Icon } from '#/lib/icons'
import { cn } from '#/lib/utils'
import type { Connection, Environment, Workspace } from '#/lib/api/types'
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from '#/components/ui/resizable'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '#/components/ui/dropdown-menu'
import { Tip } from './schema-diagram/Tip'
import { ConnectionRow, ConnectionStatusDot, EnvironmentRow } from './DatabasePanel'
import { DriverBadge } from './DriverBadge'
import { SchemaTree } from './SchemaTree'
import { useSchemaRefresh } from './useSchemaRefresh'
import { useIde, resolveConnectionState } from './useIdeStore'

/**
 * Split layout for the explorer sidebar: a top pane of connections (flat or
 * grouped by environment, with its own name search and environment filter)
 * and a bottom pane showing the schema of whichever connection is selected
 * above. Keeps a huge connection list from burying schema browsing in
 * scroll, and vice versa.
 */
export function ExplorerSplitView({
  orgSlug,
  workspace: _workspace,
  environments,
  connections,
  connectionsLoading,
  connectionsError,
  onRetry,
  connectedIds,
  canEditConnection,
  canDeleteConnection,
  selectedConnectionId,
  onSelectConnection,
  onOpenTab,
  onOpenConsole,
  onConnect,
  onDisconnect,
  groupByEnvironment,
  onAddConnection,
  onRenameEnvironment,
  onDeleteEnvironment,
  topPanelRef,
  bottomPanelRef,
  topCollapsed,
  onToggleTopPanel,
}: {
  orgSlug: string
  workspace: Workspace
  environments: Environment[]
  connections: Connection[]
  connectionsLoading: boolean
  connectionsError: boolean
  onRetry: () => void
  connectedIds: Set<number>
  canEditConnection: boolean
  canDeleteConnection: boolean
  selectedConnectionId: number | null
  onSelectConnection: (connectionId: number) => void
  onOpenTab: (connection: Connection) => void
  onOpenConsole: (connection: Connection) => void
  onConnect: (connection: Connection) => void
  onDisconnect: (connection: Connection) => void
  groupByEnvironment: boolean
  onAddConnection: (environment: Environment) => void
  onRenameEnvironment: (environment: Environment) => void
  onDeleteEnvironment: (environment: Environment) => void
  topPanelRef: RefObject<PanelImperativeHandle | null>
  bottomPanelRef: RefObject<PanelImperativeHandle | null>
  topCollapsed: boolean
  onToggleTopPanel: () => void
}) {
  const [connectionFilter, setConnectionFilter] = useState('')
  const [schemaFilter, setSchemaFilter] = useState('')
  const [envFilter, setEnvFilter] = useState<number | 'all'>('all')

  const envNameById = (id: number) => environments.find((e) => e.id === id)?.name ?? ''

  const matchesFilter = (conn: Connection) =>
    !connectionFilter || conn.name.toLowerCase().includes(connectionFilter.toLowerCase())

  const filteredConnections = connections.filter((conn) => {
    if (envFilter !== 'all' && conn.environment_id !== envFilter) return false
    return matchesFilter(conn)
  })

  const selectedConnection = connections.find((c) => c.id === selectedConnectionId) ?? null

  return (
    <ResizablePanelGroup orientation="vertical" className="min-h-0 flex-1">
      <ResizablePanel
        panelRef={topPanelRef}
        defaultSize="55%"
        minSize="20%"
        collapsible
        collapsedSize="0%"
        className="flex flex-col overflow-hidden"
      >
        <div className="flex items-center gap-1.5 border-b border-border p-2">
          <SearchInput
            value={connectionFilter}
            onValueChange={setConnectionFilter}
            onClear={() => setConnectionFilter('')}
            placeholder="Filter connections…"
            className="min-w-0 flex-1"
            size="sm"
            variant="muted"
          />
          {!groupByEnvironment && environments.length > 0 && (
            <DropdownMenu>
              <Tip
                label={
                  envFilter === 'all'
                    ? 'Filter by environment'
                    : `Environment: ${envNameById(envFilter)}`
                }
              >
                <DropdownMenuTrigger
                  render={
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon-sm"
                      aria-label="Filter by environment"
                      className={cn(
                        'size-7',
                        envFilter !== 'all' &&
                          'bg-primary/10 text-primary hover:bg-primary/15 hover:text-primary',
                      )}
                    >
                      <Icon name="server-stack-01" size={14} />
                    </Button>
                  }
                />
              </Tip>
              <DropdownMenuContent align="end" className="min-w-44">
                <DropdownMenuGroup>
                  <DropdownMenuItem onClick={() => setEnvFilter('all')}>
                    <span className="min-w-0 flex-1 truncate">All environments</span>
                    {envFilter === 'all' && (
                      <Icon name="tick-02" size={14} className="text-primary" />
                    )}
                  </DropdownMenuItem>
                  {environments.map((env) => (
                    <DropdownMenuItem key={env.id} onClick={() => setEnvFilter(env.id)}>
                      <span className="min-w-0 flex-1 truncate">{env.name}</span>
                      {envFilter === env.id && (
                        <Icon name="tick-02" size={14} className="text-primary" />
                      )}
                    </DropdownMenuItem>
                  ))}
                </DropdownMenuGroup>
              </DropdownMenuContent>
            </DropdownMenu>
          )}
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto overflow-x-hidden [scrollbar-width:thin]">
          <div className="flex flex-col py-1">
            {connectionsLoading ? (
              <SidebarMessage>Loading...</SidebarMessage>
            ) : connectionsError ? (
              <SidebarMessage>
                <span>Failed to load connections.</span>
                <button
                  type="button"
                  onClick={onRetry}
                  className="font-medium text-primary hover:underline"
                >
                  Retry
                </button>
              </SidebarMessage>
            ) : groupByEnvironment ? (
              environments.length === 0 ? (
                <SidebarMessage>No environments available.</SidebarMessage>
              ) : (
                environments.map((env) => (
                  <EnvironmentRow
                    key={env.id}
                    environment={env}
                    connections={connections.filter(
                      (c) => c.environment_id === env.id && matchesFilter(c),
                    )}
                    connectedIds={connectedIds}
                    selectedConnectionId={selectedConnectionId}
                    orgSlug={orgSlug}
                    filter=""
                    canEditConnection={canEditConnection}
                    canDeleteConnection={canDeleteConnection}
                    onSelect={onSelectConnection}
                    onOpenTab={onOpenTab}
                    onOpenConsole={onOpenConsole}
                    onConnect={onConnect}
                    onDisconnect={onDisconnect}
                    onAddConnection={() => onAddConnection(env)}
                    onRenameEnvironment={() => onRenameEnvironment(env)}
                    onDeleteEnvironment={() => onDeleteEnvironment(env)}
                    wholeRowClickable
                  />
                ))
              )
            ) : filteredConnections.length === 0 ? (
              <SidebarMessage>No connections.</SidebarMessage>
            ) : (
              filteredConnections.map((conn) => (
                <ConnectionRow
                  key={conn.id}
                  connection={conn}
                  isConnected={connectedIds.has(conn.id)}
                  selected={selectedConnectionId === conn.id}
                  connIndent={0}
                  envLabel={envFilter === 'all' ? envNameById(conn.environment_id) : undefined}
                  orgSlug={orgSlug}
                  filter=""
                  canEditConnection={canEditConnection}
                  canDeleteConnection={canDeleteConnection}
                  hideSchemaExpand
                  wholeRowClickable
                  onSelect={() => onSelectConnection(conn.id)}
                  onOpenTab={() => onOpenTab(conn)}
                  onOpenConsole={() => onOpenConsole(conn)}
                  onConnect={() => onConnect(conn)}
                  onDisconnect={() => onDisconnect(conn)}
                />
              ))
            )}
          </div>
        </div>
      </ResizablePanel>
      <ResizableHandle withHandle />
      <ResizablePanel
        panelRef={bottomPanelRef}
        defaultSize="45%"
        minSize="20%"
        collapsible
        collapsedSize="0%"
        className="flex flex-col overflow-hidden"
      >
        {selectedConnection ? (
          <SchemaPane
            orgSlug={orgSlug}
            connection={selectedConnection}
            schemaFilter={schemaFilter}
            onSchemaFilterChange={setSchemaFilter}
            onConnect={() => onConnect(selectedConnection)}
            maximizeLabel={topCollapsed ? 'Restore connections panel' : 'Maximize schema panel'}
            maximizeIcon={topCollapsed ? 'minimize' : 'maximize'}
            onToggleMaximize={onToggleTopPanel}
          />
        ) : (
          <SidebarMessage>Select a connection to browse its schema.</SidebarMessage>
        )}
      </ResizablePanel>
    </ResizablePanelGroup>
  )
}

function SchemaPane({
  orgSlug,
  connection,
  schemaFilter,
  onSchemaFilterChange,
  onConnect,
  maximizeLabel,
  maximizeIcon,
  onToggleMaximize,
}: {
  orgSlug: string
  connection: Connection
  schemaFilter: string
  onSchemaFilterChange: (value: string) => void
  onConnect: () => void
  maximizeLabel: string
  maximizeIcon: 'maximize' | 'minimize'
  onToggleMaximize: () => void
}) {
  const sessionId = useIde((s) => s.sessions[connection.id])
  const connStatus = useIde((s) => s.connectionStatus[connection.id])
  const connState = resolveConnectionState(Boolean(sessionId), connStatus)
  const refresh = useSchemaRefresh({
    orgSlug,
    workspaceId: connection.workspace_id,
    connectionId: connection.id,
    sessionId,
  })

  return (
    <>
      <div className="flex h-8 shrink-0 items-center gap-2 border-b border-border px-2 text-xs font-medium">
        <span className="relative shrink-0">
          <DriverBadge driver={connection.driver} size="sm" />
          <ConnectionStatusDot state={connState} />
        </span>
        <span className="min-w-0 flex-1 truncate" title={connection.name}>
          {connection.name}
        </span>
        {sessionId && (
          <Tip label="Refresh schema">
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label="Refresh schema"
              disabled={refresh.isPending}
              className="size-6"
              onClick={() => refresh.mutate()}
            >
              <Icon
                name="refresh"
                size={13}
                className={refresh.isPending ? 'animate-spin' : undefined}
              />
            </Button>
          </Tip>
        )}
        <Tip label={maximizeLabel}>
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label={maximizeLabel}
            className="size-6"
            onClick={onToggleMaximize}
          >
            <Icon name={maximizeIcon} size={13} />
          </Button>
        </Tip>
      </div>
      <div className="border-b border-border p-2">
        <SearchInput
          value={schemaFilter}
          onValueChange={onSchemaFilterChange}
          onClear={() => onSchemaFilterChange('')}
          placeholder="Filter schema…"
          size="sm"
          variant="muted"
        />
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto overflow-x-hidden [scrollbar-width:thin]">
        <SchemaTree
          orgSlug={orgSlug}
          workspaceId={connection.workspace_id}
          connectionId={connection.id}
          driver={connection.driver}
          filter={schemaFilter}
          onConnect={onConnect}
        />
      </div>
    </>
  )
}

function SidebarMessage({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex flex-col items-center gap-1 px-3 py-6 text-center text-xs text-muted-foreground">
      {children}
    </div>
  )
}
