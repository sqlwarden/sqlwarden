import { useState, useEffect } from 'react'
import * as Y from 'yjs'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { HugeiconsIcon } from '@hugeicons/react'
import {
  ArrowDown01Icon,
  ArrowRight01Icon,
  Cancel01Icon,
  DatabaseIcon,
  FlowConnectionIcon,
  PlusSignIcon,
  ServerStack01Icon,
  TerminalIcon,
} from '@hugeicons/core-free-icons'
import { toast } from 'sonner'
import {
  orgEffectivePermissionsQueryOptions,
  orgEnvironmentsQueryOptions,
  orgWorkspaceConnectionsQueryOptions,
} from '#/lib/api/query'
import { api } from '#/lib/api/client'
import { isApiError } from '#/lib/api/errors'
import type { Connection, Environment, Workspace } from '#/lib/api/types'
import { cn } from '#/lib/utils'
import { hasPermission, permission } from '#/lib/permissions'
import { useIde, newConnectionTab, DEFAULT_CONSOLE_CONTENT } from './useIdeStore'
import { SidebarPane } from './SidebarPane'
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
  maximized: boolean
  onMaximizedChange: (maximized: boolean) => void
}

export function DatabasePanel({ orgSlug, workspace, maximized, onMaximizedChange }: DatabasePanelProps) {
  const openTab = useIde((s) => s.openTab)
  const openConsole = useIde((s) => s.openConsole)
  const sessions = useIde((s) => s.sessions)
  const setSession = useIde((s) => s.setSession)
  const clearSession = useIde((s) => s.clearSession)
  const syncSessions = useIde((s) => s.syncSessions)
  const queryClient = useQueryClient()

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
    onSuccess: (data, conn) => {
      setSession(conn.id, data.session_id)
      void queryClient.invalidateQueries({ queryKey: sessionsQueryKey })
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : 'Failed to connect')
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
      <HugeiconsIcon icon={PlusSignIcon} size={14} strokeWidth={2} />
    </Button>
  ) : undefined

  return (
    <>
      <SidebarPane
        title="Explorer"
        icon={DatabaseIcon}
        maximized={maximized}
        onMaximizedChange={onMaximizedChange}
        actions={actions}
      >
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
              onOpen={handleOpenConnection}
              onOpenConsole={handleOpenConsole}
              onConnect={handleConnect}
              onDisconnect={handleDisconnect}
              onAddConnection={() => setAddConnEnvironmentId(env.id)}
            />
          ))
        )}
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
  onOpen: (conn: Connection) => void
  onOpenConsole: (conn: Connection) => void
  onConnect: (conn: Connection) => void
  onDisconnect: (conn: Connection) => void
  onAddConnection: () => void
}) {
  const [expanded, setExpanded] = useState(true)

  const envPermissions = useQuery(
    orgEffectivePermissionsQueryOptions(orgSlug, 'environment', environment.id),
  )
  const canCreateConnection = hasPermission(envPermissions.data?.permissions, permission.connCreate)

  return (
    <div>
      <div className="group flex h-7 w-full items-center text-xs transition-colors hover:bg-accent hover:text-accent-foreground">
        <button
          type="button"
          onClick={() => setExpanded((v) => !v)}
          className="flex min-w-0 flex-1 items-center gap-1.5 px-2 text-left"
        >
          <HugeiconsIcon
            icon={expanded ? ArrowDown01Icon : ArrowRight01Icon}
            size={11}
            strokeWidth={2}
            className="shrink-0 text-muted-foreground"
          />
          <HugeiconsIcon
            icon={ServerStack01Icon}
            size={14}
            strokeWidth={2}
            className="shrink-0 text-muted-foreground"
          />
          <span className="min-w-0 flex-1 truncate font-medium">{environment.name}</span>
        </button>
        {canCreateConnection && (
          <button
            type="button"
            onClick={onAddConnection}
            aria-label={`New connection in ${environment.name}`}
            className="mr-1 flex size-5 shrink-0 items-center justify-center rounded opacity-0 transition-opacity hover:bg-muted group-hover:opacity-100"
          >
            <HugeiconsIcon icon={PlusSignIcon} size={11} strokeWidth={2} />
          </button>
        )}
      </div>

      {expanded && (
        <div className="ml-[18px] border-l border-border">
          {connections.length === 0 ? (
            <div className="px-3 py-1.5 text-xs text-muted-foreground">No connections.</div>
          ) : (
            connections.map((conn) => (
              <ConnectionRow
                key={conn.id}
                connection={conn}
                isConnected={connectedIds.has(conn.id)}
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
  onOpen,
  onOpenConsole,
  onConnect,
  onDisconnect,
}: {
  connection: Connection
  isConnected: boolean
  onOpen: () => void
  onOpenConsole: () => void
  onConnect: () => void
  onDisconnect: () => void
}) {
  const [menuOpen, setMenuOpen] = useState(false)
  const [pos, setPos] = useState({ x: 0, y: 0 })

  function handleContextMenu(e: React.MouseEvent) {
    e.preventDefault()
    e.stopPropagation()
    setPos({ x: e.clientX, y: e.clientY })
    setMenuOpen(true)
  }

  return (
    <>
      <div
        onContextMenu={handleContextMenu}
        className={menuOpen ? 'bg-accent text-accent-foreground' : ''}
      >
        <button
          type="button"
          onClick={onOpen}
          className={cn(
            'flex h-7 w-full items-center gap-2 px-3 text-left text-xs',
            'transition-colors hover:bg-accent hover:text-accent-foreground',
          )}
        >
          <DriverBadge driver={connection.driver} size="sm" />
          <span className="min-w-0 flex-1 truncate">{connection.name}</span>
          {isConnected && (
            <div className="size-1.5 shrink-0 rounded-full bg-green-500" />
          )}
        </button>

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
              <HugeiconsIcon icon={FlowConnectionIcon} size={13} strokeWidth={2} />
              Open
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => { setMenuOpen(false); onOpenConsole() }}>
              <HugeiconsIcon icon={TerminalIcon} size={13} strokeWidth={2} />
              Open Console
            </DropdownMenuItem>
            {isConnected ? (
              <DropdownMenuItem
                data-variant="destructive"
                onClick={() => { setMenuOpen(false); onDisconnect() }}
              >
                <HugeiconsIcon icon={Cancel01Icon} size={13} strokeWidth={2} />
                Disconnect
              </DropdownMenuItem>
            ) : (
              <DropdownMenuItem onClick={() => { setMenuOpen(false); onConnect() }}>
                <HugeiconsIcon icon={FlowConnectionIcon} size={13} strokeWidth={2} />
                Connect
              </DropdownMenuItem>
            )}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </>
  )
}

function SidebarMessage({ children }: { children: React.ReactNode }) {
  return <div className="px-3 py-3 text-xs text-muted-foreground">{children}</div>
}
