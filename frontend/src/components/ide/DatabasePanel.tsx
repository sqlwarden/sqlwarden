import { useState, useEffect } from 'react'
import * as Y from 'yjs'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Icon } from '#/lib/icons'
import { toast } from 'sonner'
import {
  orgEffectivePermissionsQueryOptions,
  orgEnvironmentsQueryOptions,
  orgWorkspaceConnectionsQueryOptions,
  refreshConnectionSchema,
  connectionSchemaQueryKey,
} from '#/lib/api/query'
import { api } from '#/lib/api/client'
import { isApiError } from '#/lib/api/errors'
import type { Connection, Environment, Workspace } from '#/lib/api/types'
import { cn } from '#/lib/utils'
import { hasPermission, permission } from '#/lib/permissions'
import { useIde, activeTabId as selectActiveTabId, resolveConnectionState, type ConnectionState, newConnectionTab, DEFAULT_CONSOLE_CONTENT } from './useIdeStore'
import { SidebarPane } from './SidebarPane'
import { SchemaTree } from './SchemaTree'
import { ConnectionDialog } from './ConnectionDialog'
import { DriverBadge } from './DriverBadge'
import { Button } from '#/components/ui/button'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '#/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '#/components/ui/dropdown-menu'
import { Input } from '#/components/ui/input'
import { Label } from '#/components/ui/label'
import { Textarea } from '#/components/ui/textarea'

type DatabasePanelProps = {
  orgSlug: string
  workspace: Workspace
  maximized?: boolean
  onMaximizedChange?: (maximized: boolean) => void
}

export function DatabasePanel({ orgSlug, workspace, maximized, onMaximizedChange }: DatabasePanelProps) {
  const openTab = useIde((s) => s.openTab)
  const openConsole = useIde((s) => s.openConsole)
  const sessions = useIde((s) => s.sessions)
  const setSession = useIde((s) => s.setSession)
  const clearSession = useIde((s) => s.clearSession)
  const syncSessions = useIde((s) => s.syncSessions)
  const setConnectionStatus = useIde((s) => s.setConnectionStatus)
  const queryClient = useQueryClient()

  const [filter, setFilter] = useState('')
  const [addEnvOpen, setAddEnvOpen] = useState(false)
  const [addConnEnvironmentId, setAddConnEnvironmentId] = useState<number | null>(null)
  const [envName, setEnvName] = useState('')
  const [envDescription, setEnvDescription] = useState('')
  const [envNameError, setEnvNameError] = useState<string | undefined>(undefined)

  const effectivePermissions = useQuery(
    orgEffectivePermissionsQueryOptions(orgSlug, 'workspace', workspace.id),
  )
  const canCreateEnvironment = hasPermission(effectivePermissions.data?.permissions, permission.envCreate)

  const environments = useQuery(
    orgEnvironmentsQueryOptions(orgSlug, workspace.id, { page_size: 100, sort: 'name', order: 'asc' }),
  )
  const connections = useQuery(
    orgWorkspaceConnectionsQueryOptions(orgSlug, workspace.id, { page_size: 100, sort: 'name', order: 'asc' }),
  )

  // Authoritative session list from the backend — reconciles persisted frontend
  // state with what the server actually has alive (handles restarts, TTL expiry, etc).
  const backendSessionsQuery = useQuery({
    queryKey: ['org-workspace-sessions', orgSlug, workspace.id],
    queryFn: () =>
      api.get<{ sessions: { connection_id: number; session_id: string }[] }>(
        `/api/v1/orgs/${orgSlug}/workspaces/${workspace.id}/sessions`,
      ),
    staleTime: 0,
    refetchOnWindowFocus: true,
  })

  useEffect(() => {
    if (!backendSessionsQuery.data) return
    const map: Record<number, string> = {}
    for (const s of backendSessionsQuery.data.sessions) {
      map[s.connection_id] = s.session_id
    }
    syncSessions(map)
  }, [backendSessionsQuery.data, syncSessions])

  const envItems = environments.data?.items ?? []
  const connItems = connections.data?.items ?? []

  const connectedIds = new Set(Object.keys(sessions).map(Number))

  const sessionsQueryKey = ['org-workspace-sessions', orgSlug, workspace.id]

  const connectMutation = useMutation({
    mutationFn: (conn: Connection) =>
      api.post<{ session_id: string; reused: boolean }>(
        `/api/v1/orgs/${orgSlug}/workspaces/${workspace.id}/connections/${conn.id}/connect`,
      ),
    onMutate: (conn) => {
      setConnectionStatus(conn.id, 'connecting')
    },
    onSuccess: (data, conn) => {
      setConnectionStatus(conn.id, null)
      setSession(conn.id, data.session_id)
      void queryClient.invalidateQueries({ queryKey: sessionsQueryKey })
    },
    onError: (error, conn) => {
      const message = error instanceof Error ? error.message : 'Failed to connect'
      setConnectionStatus(conn.id, { error: message })
      toast.error(message)
    },
  })

  const disconnectMutation = useMutation({
    mutationFn: ({ conn, sessionId }: { conn: Connection; sessionId: string }) =>
      api.delete(
        `/api/v1/orgs/${orgSlug}/workspaces/${workspace.id}/connections/${conn.id}/session`,
        { headers: { 'X-Warden-Session': sessionId } },
      ),
    onSuccess: (_, { conn }) => {
      clearSession(conn.id)
      setConnectionStatus(conn.id, null)
      void queryClient.invalidateQueries({ queryKey: sessionsQueryKey })
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : 'Failed to disconnect')
    },
  })

  function handleOpenConnection(conn: Connection) {
    openTab(newConnectionTab(conn, workspace))
  }

  function handleOpenConsole(conn: Connection) {
    const tmpDoc = new Y.Doc()
    tmpDoc.getText('content').insert(0, DEFAULT_CONSOLE_CONTENT)
    const yState = Array.from(Y.encodeStateAsUpdate(tmpDoc))
    tmpDoc.destroy()
    openConsole(workspace, yState, conn.id)
  }

  function handleConnect(conn: Connection) {
    void connectMutation.mutateAsync(conn).catch(() => {})
  }

  function handleDisconnect(conn: Connection) {
    const sessionId = sessions[conn.id]
    if (!sessionId) return
    void disconnectMutation.mutateAsync({ conn, sessionId }).catch(() => {})
  }

  const createEnvironment = useMutation({
    mutationFn: async () =>
      api.post<Environment>(`/api/v1/orgs/${orgSlug}/workspaces/${workspace.id}/environments`, {
        name: envName.trim(),
        description: envDescription.trim(),
      }),
    onSuccess: async () => {
      setAddEnvOpen(false)
      setEnvName('')
      setEnvDescription('')
      setEnvNameError(undefined)
      toast.success('Environment created')
      await queryClient.invalidateQueries({ queryKey: ['org-environments', orgSlug] })
    },
    onError: (error) => {
      if (isApiError(error) && error.fieldErrors) {
        setEnvNameError(error.fieldErrors.name)
        if (error.fieldErrors.name) return
      }
      toast.error(error instanceof Error ? error.message : 'Failed to create environment')
    },
  })

  function handleAddEnvSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!envName.trim()) {
      setEnvNameError('Name is required.')
      return
    }
    setEnvNameError(undefined)
    void createEnvironment.mutateAsync().catch(() => {})
  }

  const actions = canCreateEnvironment ? (
    <Button
      type="button"
      variant="ghost"
      size="icon-sm"
      aria-label="New Environment"
      onClick={() => setAddEnvOpen(true)}
    >
      <Icon name="plus-sign" size={14} />
    </Button>
  ) : undefined

  return (
    <>
      <SidebarPane
        title="Explorer"
        icon="server-stack-01"
        maximized={maximized}
        onMaximizedChange={onMaximizedChange}
        actions={actions}
        scroll={false}
      >
        <div className="border-b border-border p-1.5">
          <Input
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            placeholder="Filter schema…"
            className="h-7 text-xs"
          />
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto overflow-x-hidden [scrollbar-width:thin]">
          <div className="flex flex-col py-1">
            {environments.isLoading || connections.isLoading ? (
              <SidebarMessage>Loading...</SidebarMessage>
            ) : environments.isError || connections.isError ? (
              <SidebarMessage>Failed to load database tree.</SidebarMessage>
            ) : envItems.length === 0 ? (
              <SidebarMessage>No environments available.</SidebarMessage>
            ) : (
              envItems.map((env) => (
                <EnvironmentRow
                  key={env.id}
                  environment={env}
                  connections={connItems.filter((c) => c.environment_id === env.id)}
                  connectedIds={connectedIds}
                  orgSlug={orgSlug}
                  filter={filter}
                  onOpen={handleOpenConnection}
                  onOpenConsole={handleOpenConsole}
                  onConnect={handleConnect}
                  onDisconnect={handleDisconnect}
                  onAddConnection={() => setAddConnEnvironmentId(env.id)}
                />
              ))
            )}
          </div>
        </div>
      </SidebarPane>

      <Dialog
        open={addEnvOpen}
        onOpenChange={(open) => {
          setAddEnvOpen(open)
          if (!open) {
            setEnvName('')
            setEnvDescription('')
            setEnvNameError(undefined)
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>New Environment</DialogTitle>
          </DialogHeader>
          <form className="mt-6 flex flex-col gap-4" onSubmit={handleAddEnvSubmit}>
            <div className="flex flex-col gap-2">
              <Label>Name</Label>
              <Input
                value={envName}
                disabled={createEnvironment.isPending}
                aria-invalid={envNameError ? true : undefined}
                onChange={(e) => {
                  setEnvName(e.target.value)
                  setEnvNameError(undefined)
                }}
              />
              {envNameError ? <p className="text-xs text-destructive">{envNameError}</p> : null}
            </div>
            <div className="flex flex-col gap-2">
              <Label>Description</Label>
              <Textarea
                value={envDescription}
                disabled={createEnvironment.isPending}
                placeholder="Optional environment description"
                onChange={(e) => setEnvDescription(e.target.value)}
              />
            </div>
            <DialogFooter>
              <DialogClose render={<Button type="button" variant="ghost" disabled={createEnvironment.isPending} />}>
                Cancel
              </DialogClose>
              <Button type="submit" disabled={createEnvironment.isPending}>
                {createEnvironment.isPending ? 'Creating...' : 'Create'}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <ConnectionDialog
        open={addConnEnvironmentId !== null}
        onOpenChange={(open) => { if (!open) setAddConnEnvironmentId(null) }}
        orgSlug={orgSlug}
        workspaceId={workspace.id}
        environments={envItems}
        lockedEnvironmentId={addConnEnvironmentId ?? undefined}
      />
    </>
  )
}

function EnvironmentRow({
  environment,
  connections,
  connectedIds,
  orgSlug,
  filter,
  onOpen,
  onOpenConsole,
  onConnect,
  onDisconnect,
  onAddConnection,
}: {
  environment: Environment
  connections: Connection[]
  connectedIds: Set<number>
  orgSlug: string
  filter: string
  onOpen: (conn: Connection) => void
  onOpenConsole: (conn: Connection) => void
  onConnect: (conn: Connection) => void
  onDisconnect: (conn: Connection) => void
  onAddConnection: () => void
}) {
  const [expanded, setExpanded] = useState(connections.length > 0)

  const envPermissions = useQuery(
    orgEffectivePermissionsQueryOptions(orgSlug, 'environment', environment.id),
  )
  const canCreateConnection = hasPermission(envPermissions.data?.permissions, permission.connCreate)

  return (
    <div>
      <div className="group flex h-6 w-full items-center text-xs transition-colors hover:bg-accent hover:text-accent-foreground">
        <button
          type="button"
          onClick={() => setExpanded((v) => !v)}
          className="flex min-w-0 flex-1 items-center gap-1.5 px-2 text-left"
        >
          <Icon
            name={expanded ? 'chevron-down' : 'chevron-right'}
            size={11}
            className="shrink-0 text-muted-foreground"
          />
          <Icon
            name="box"
            size={13}
            className="shrink-0 text-muted-foreground"
          />
          <span className="min-w-0 flex-1 truncate font-medium" title={environment.name}>{environment.name}</span>
        </button>
        {canCreateConnection && (
          <button
            type="button"
            onClick={onAddConnection}
            aria-label={`New connection in ${environment.name}`}
            className="mr-1 flex size-5 shrink-0 items-center justify-center rounded opacity-0 transition-opacity hover:bg-muted group-hover:opacity-100"
          >
            <Icon name="plus-sign" size={11} />
          </button>
        )}
      </div>

      {expanded && (
        <div>
          {connections.length === 0 ? (
            <div className="py-1.5 pl-[18px] pr-2 text-xs text-muted-foreground">No connections.</div>
          ) : (
            connections.map((conn) => (
              <ConnectionRow
                key={conn.id}
                connection={conn}
                isConnected={connectedIds.has(conn.id)}
                orgSlug={orgSlug}
                filter={filter}
                onOpen={() => onOpen(conn)}
                onOpenConsole={() => onOpenConsole(conn)}
                onConnect={() => onConnect(conn)}
                onDisconnect={() => onDisconnect(conn)}
              />
            ))
          )}
        </div>
      )}
    </div>
  )
}

function ConnectionRow({
  connection,
  isConnected,
  orgSlug,
  filter,
  onOpen,
  onOpenConsole,
  onConnect,
  onDisconnect,
}: {
  connection: Connection
  isConnected: boolean
  orgSlug: string
  filter: string
  onOpen: () => void
  onOpenConsole: () => void
  onConnect: () => void
  onDisconnect: () => void
}) {
  const [menuOpen, setMenuOpen] = useState(false)
  const [expanded, setExpanded] = useState(false)
  const [pos, setPos] = useState({ x: 0, y: 0 })
  const sessionId = useIde((s) => s.sessions[connection.id])
  const connStatus = useIde((s) => s.connectionStatus[connection.id])
  const connState = resolveConnectionState(Boolean(sessionId), connStatus)
  // Hint the connection used by the active tab (file's linked connection, console, or connection tab).
  const isActive = useIde((s) => {
    const id = selectActiveTabId(s, connection.workspace_id)
    return s.tabs.find((t) => t.id === id)?.connectionId === connection.id
  })
  const queryClient = useQueryClient()
  const refresh = useMutation({
    mutationFn: () => refreshConnectionSchema(orgSlug, connection.workspace_id, connection.id, sessionId ?? ''),
    onSuccess: () =>
      queryClient.invalidateQueries({
        queryKey: connectionSchemaQueryKey(orgSlug, connection.workspace_id, connection.id),
      }),
  })

  function handleContextMenu(e: React.MouseEvent) {
    e.preventDefault()
    e.stopPropagation()
    setPos({ x: e.clientX, y: e.clientY })
    setMenuOpen(true)
  }

  return (
    <div>
      <div
        onContextMenu={handleContextMenu}
        className={cn(
          'flex items-center pl-[18px] transition-colors',
          isActive
            ? 'bg-primary/10 hover:bg-primary/15'
            : 'hover:bg-accent hover:text-accent-foreground',
          menuOpen && 'bg-accent text-accent-foreground',
        )}
      >
        {isConnected ? (
          <button
            type="button"
            aria-label={expanded ? 'Collapse schema' : 'Expand schema'}
            onClick={() => setExpanded((v) => !v)}
            className="flex h-6 w-5 shrink-0 items-center justify-center text-muted-foreground hover:text-foreground"
          >
            <Icon name={expanded ? 'chevron-down' : 'chevron-right'} size={11} />
          </button>
        ) : (
          <span className="w-5 shrink-0" />
        )}

        <button
          type="button"
          onClick={onOpen}
          className="flex h-6 min-w-0 items-center gap-2 text-left text-xs"
        >
          <span className="relative shrink-0">
            <DriverBadge driver={connection.driver} size="sm" />
            <ConnectionStatusDot state={connState} />
          </span>
          <span className="truncate" title={connection.name}>{connection.name}</span>
        </button>

        {isConnected && (
          <button
            type="button"
            aria-label="Refresh schema"
            disabled={refresh.isPending}
            onClick={(e) => {
              e.stopPropagation()
              refresh.mutate()
            }}
            className="ml-1 flex size-5 shrink-0 items-center justify-center rounded text-muted-foreground transition-colors hover:text-foreground disabled:opacity-50"
          >
            <Icon name="refresh" size={11} className={refresh.isPending ? 'animate-spin' : undefined} />
          </button>
        )}
        <div className="h-6 flex-1" />

        <DropdownMenu open={menuOpen} onOpenChange={setMenuOpen}>
          <DropdownMenuTrigger
            nativeButton={false}
            render={
              <span
                style={{
                  position: 'fixed',
                  left: pos.x,
                  top: pos.y,
                  width: 0,
                  height: 0,
                  pointerEvents: 'none',
                }}
              />
            }
          />
          <DropdownMenuContent align="start" side="bottom" sideOffset={2} className="w-48">
            <DropdownMenuItem onClick={() => { setMenuOpen(false); onOpen() }}>
              <Icon name="flow-connection" size={13} />
              Open
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => { setMenuOpen(false); onOpenConsole() }}>
              <Icon name="terminal" size={13} />
              Open Console
            </DropdownMenuItem>
            {isConnected ? (
              <DropdownMenuItem
                data-variant="destructive"
                onClick={() => { setMenuOpen(false); onDisconnect() }}
              >
                <Icon name="cancel-01" size={13} />
                Disconnect
              </DropdownMenuItem>
            ) : (
              <DropdownMenuItem onClick={() => { setMenuOpen(false); onConnect() }}>
                <Icon name="flow-connection" size={13} />
                Connect
              </DropdownMenuItem>
            )}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      {isConnected && expanded && (
        <div className="ml-[36px] border-l border-border">
          <SchemaTree
            orgSlug={orgSlug}
            workspaceId={connection.workspace_id}
            connectionId={connection.id}
            driver={connection.driver}
            filter={filter}
          />
        </div>
      )}
    </div>
  )
}

function ConnectionStatusDot({ state }: { state: ConnectionState }) {
  if (state.kind === 'idle') return null
  if (state.kind === 'connecting') {
    return (
      <Icon
        name="loading-03"
        size={11}
        className="absolute -bottom-1 -right-1 animate-spin text-amber-500"
      />
    )
  }
  const color = state.kind === 'connected' ? 'bg-green-500' : 'bg-red-500'
  return (
    <span
      title={state.kind === 'error' ? state.message : undefined}
      className={cn('absolute -bottom-0.5 -right-0.5 size-1.5 rounded-full ring-1 ring-sidebar', color)}
    />
  )
}

function SidebarMessage({ children }: { children: React.ReactNode }) {
  return <div className="px-3 py-3 text-xs text-muted-foreground">{children}</div>
}
