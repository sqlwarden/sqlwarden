import { errorMessage } from '#/lib/api/errors'
import { useState } from 'react'
import { queryKeys } from '#/lib/api/query-keys'
import { useNavigate } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Icon } from '#/lib/icons'
import { toast } from 'sonner'
import {
  orgEffectivePermissionsQueryOptions,
  orgEnvironmentsQueryOptions,
  allOrgWorkspaceConnectionsQueryOptions,
} from '#/lib/api/query'
import { api } from '#/lib/api/client'
import { isApiError } from '#/lib/api/errors'
import type { Connection, Environment, Workspace } from '#/lib/api/types'
import { cn } from '#/lib/utils'
import { hasPermission, permission } from '#/lib/permissions'
import {
  useIde,
  activeTabId as selectActiveTabId,
  resolveConnectionState,
  type ConnectionState,
} from './useIdeStore'
import { useConnectionLayout } from './useConnectionLayout'
import { ContextMenu } from '#/components/ui/context-menu'
import { copyWithToast } from './contextMenus/clipboard'
import { buildConnectionMenu } from './contextMenus/connectionMenu'
import { buildEnvironmentMenu } from './contextMenus/environmentMenu'
import { SidebarPane } from './SidebarPane'
import { SchemaTree } from './SchemaTree'
import { sidebarActiveRowClass } from './sidebarRowStyles'
import { ConnectionDialog } from './ConnectionDialog'
import { EditConnectionDialog } from './EditConnectionDialog'
import { DriverBadge } from './DriverBadge'
import { Button } from '#/components/ui/button'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '#/components/ui/alert-dialog'
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
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '#/components/ui/dropdown-menu'
import { Tip } from './schema-diagram/Tip'
import { Input } from '#/components/ui/input'
import { Label } from '#/components/ui/label'
import { Textarea } from '#/components/ui/textarea'
import { Field, FieldError, FieldGroup, FieldLabel } from '#/components/ui/field'
import { useConnectionActions } from './useConnectionActions'
import { useSchemaRefresh } from './useSchemaRefresh'
import { TransactionGuardDialog } from './TransactionGuardDialog'
import { useTransactionMode } from './useTransactionMode'

type DatabasePanelProps = {
  orgSlug: string
  workspace: Workspace
  maximized?: boolean
  onMaximizedChange?: (maximized: boolean) => void
}

export function DatabasePanel({
  orgSlug,
  workspace,
  maximized,
  onMaximizedChange,
}: DatabasePanelProps) {
  const queryClient = useQueryClient()
  const connectionActions = useConnectionActions(orgSlug, workspace)

  const [filter, setFilter] = useState('')
  const { connectionLayout: connLayout } = useConnectionLayout()
  const [envFilter, setEnvFilter] = useState<number | 'all'>('all')
  const [addEnvOpen, setAddEnvOpen] = useState(false)
  const [addConnEnvironmentId, setAddConnEnvironmentId] = useState<number | null>(null)
  const [addConnOpen, setAddConnOpen] = useState(false)
  const [envName, setEnvName] = useState('')
  const [envDescription, setEnvDescription] = useState('')
  const [envNameError, setEnvNameError] = useState<string | undefined>(undefined)
  const [renamingEnvironment, setRenamingEnvironment] = useState<Environment | null>(null)
  const [renameName, setRenameName] = useState('')
  const [renameNameError, setRenameNameError] = useState<string | undefined>(undefined)
  const [deletingEnvironment, setDeletingEnvironment] = useState<Environment | null>(null)
  const [blockedDisconnectConnection, setBlockedDisconnectConnection] = useState<Connection | null>(
    null,
  )

  const transactions = useIde((s) => s.transactions)
  const blockedDisconnectSessionId = useIde((s) =>
    blockedDisconnectConnection ? s.sessions[blockedDisconnectConnection.id] : undefined,
  )
  const blockedDisconnectTransaction = useTransactionMode(
    orgSlug,
    workspace.id,
    blockedDisconnectConnection?.id,
    blockedDisconnectSessionId,
  )

  function handleDisconnect(conn: Connection) {
    const transactionOpen = Boolean(transactions[conn.id]?.open)
    connectionActions.disconnect(conn, transactionOpen, () => setBlockedDisconnectConnection(conn))
  }

  const effectivePermissions = useQuery(
    orgEffectivePermissionsQueryOptions(orgSlug, 'workspace', workspace.id),
  )
  const canCreateEnvironment = hasPermission(
    effectivePermissions.data?.permissions,
    permission.envCreate,
  )
  const canCreateConnectionInWorkspace = hasPermission(
    effectivePermissions.data?.permissions,
    permission.connCreate,
  )
  const canEditConnection = hasPermission(
    effectivePermissions.data?.permissions,
    permission.connUpdate,
  )
  const canDeleteConnection = hasPermission(
    effectivePermissions.data?.permissions,
    permission.connDelete,
  )

  const environments = useQuery(
    orgEnvironmentsQueryOptions(orgSlug, workspace.id, {
      page_size: 100,
      sort: 'name',
      order: 'asc',
    }),
  )
  const connections = useQuery(allOrgWorkspaceConnectionsQueryOptions(orgSlug, workspace.id))

  const envItems = environments.data?.items ?? []
  const connItems = connections.data?.items ?? []
  const envNameById = (id: number) => envItems.find((e) => e.id === id)?.name ?? ''

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
      await queryClient.invalidateQueries({ queryKey: queryKeys.orgEnvironmentsScope(orgSlug) })
    },
    onError: (error) => {
      if (isApiError(error) && error.fieldErrors) {
        setEnvNameError(error.fieldErrors.name)
        if (error.fieldErrors.name) return
      }
      toast.error(errorMessage(error, 'Failed to create environment'))
    },
  })

  const renameEnvironment = useMutation({
    mutationFn: ({ environment, name }: { environment: Environment; name: string }) =>
      api.patch<void>(
        `/api/v1/orgs/${orgSlug}/workspaces/${workspace.id}/environments/${environment.id}`,
        {
          name,
          description: environment.description ?? '',
        },
      ),
    onSuccess: async () => {
      setRenamingEnvironment(null)
      setRenameName('')
      setRenameNameError(undefined)
      toast.success('Environment renamed')
      await queryClient.invalidateQueries({
        queryKey: queryKeys.orgEnvironmentsScope(orgSlug, workspace.id),
      })
    },
    onError: (error) => {
      if (isApiError(error) && error.fieldErrors?.name) {
        setRenameNameError(error.fieldErrors.name)
        return
      }
      toast.error(errorMessage(error, 'Failed to rename environment'))
    },
  })

  const deleteEnvironment = useMutation({
    mutationFn: (environment: Environment) =>
      api.delete<void>(
        `/api/v1/orgs/${orgSlug}/workspaces/${workspace.id}/environments/${environment.id}`,
      ),
    onSuccess: async (_, environment) => {
      setDeletingEnvironment(null)
      setEnvFilter((current) => (current === environment.id ? 'all' : current))
      toast.success('Environment deleted')
      await queryClient.invalidateQueries({
        queryKey: queryKeys.orgEnvironmentsScope(orgSlug, workspace.id),
      })
    },
    onError: (error) => {
      toast.error(errorMessage(error, 'Failed to delete environment'))
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

  function openRenameEnvironment(environment: Environment) {
    setRenamingEnvironment(environment)
    setRenameName(environment.name)
    setRenameNameError(undefined)
  }

  function handleRenameSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!renamingEnvironment) return
    const name = renameName.trim()
    if (!name) {
      setRenameNameError('Name is required.')
      return
    }
    setRenameNameError(undefined)
    void renameEnvironment.mutateAsync({ environment: renamingEnvironment, name }).catch(() => {})
  }

  const deletingEnvironmentConnectionCount = deletingEnvironment
    ? connItems.filter((connection) => connection.environment_id === deletingEnvironment.id).length
    : 0

  // Connection creation is available from the panel header regardless of the
  // grouped/flat layout; per-environment permissions are enforced server-side.
  const canAddConnection = envItems.length > 0 && canCreateConnectionInWorkspace
  const actions =
    canAddConnection || canCreateEnvironment ? (
      <DropdownMenu>
        <Tip label="New connection or environment">
          <DropdownMenuTrigger
            render={
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                aria-label="Add connection or environment"
              >
                <Icon name="plus-sign" size={14} />
              </Button>
            }
          />
        </Tip>
        <DropdownMenuContent align="end" className="min-w-48">
          <DropdownMenuGroup>
            {canAddConnection && (
              <DropdownMenuItem onClick={() => setAddConnOpen(true)}>
                <Icon name="database" size={16} />
                New Connection…
              </DropdownMenuItem>
            )}
            {canCreateEnvironment && (
              <DropdownMenuItem onClick={() => setAddEnvOpen(true)}>
                <Icon name="server-stack-01" size={16} />
                New Environment…
              </DropdownMenuItem>
            )}
          </DropdownMenuGroup>
        </DropdownMenuContent>
      </DropdownMenu>
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
        <div className="flex items-center gap-1.5 border-b border-border p-2">
          <div className="relative min-w-0 flex-1">
            <Icon
              name="search-01"
              size={12}
              className="pointer-events-none absolute left-2 top-1/2 -translate-y-1/2 text-muted-foreground"
            />
            <Input
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              placeholder="Filter schema…"
              className="h-7 border-transparent bg-muted/60 pl-7 text-xs focus-visible:bg-background dark:bg-muted/40 dark:focus-visible:bg-input/30"
            />
            {filter && (
              <Tip label="Clear filter">
                <button
                  type="button"
                  aria-label="Clear filter"
                  onClick={() => setFilter('')}
                  className="absolute right-1.5 top-1/2 flex size-4 -translate-y-1/2 items-center justify-center rounded text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                >
                  <Icon name="cancel-01" size={10} />
                </button>
              </Tip>
            )}
          </div>
          {connLayout === 'flat' && envItems.length > 0 && (
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
                  {envItems.map((env) => (
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
            {environments.isLoading || connections.isLoading ? (
              <SidebarMessage>Loading...</SidebarMessage>
            ) : environments.isError || connections.isError ? (
              <SidebarMessage>
                <span>Failed to load connections.</span>
                <button
                  type="button"
                  onClick={() => {
                    void environments.refetch()
                    void connections.refetch()
                  }}
                  className="font-medium text-primary hover:underline"
                >
                  Retry
                </button>
              </SidebarMessage>
            ) : envItems.length === 0 ? (
              <SidebarMessage>No environments available.</SidebarMessage>
            ) : connLayout === 'grouped' ? (
              envItems.map((env) => (
                <EnvironmentRow
                  key={env.id}
                  environment={env}
                  connections={connItems.filter((c) => c.environment_id === env.id)}
                  connectedIds={connectionActions.connectedIds}
                  orgSlug={orgSlug}
                  filter={filter}
                  canEditConnection={canEditConnection}
                  canDeleteConnection={canDeleteConnection}
                  onOpen={connectionActions.openConnection}
                  onOpenConsole={connectionActions.openConnectionConsole}
                  onConnect={connectionActions.connect}
                  onDisconnect={handleDisconnect}
                  onAddConnection={() => setAddConnEnvironmentId(env.id)}
                  onRenameEnvironment={() => openRenameEnvironment(env)}
                  onDeleteEnvironment={() => setDeletingEnvironment(env)}
                />
              ))
            ) : (
              (() => {
                const list = connItems.filter(
                  (c) => envFilter === 'all' || c.environment_id === envFilter,
                )
                if (list.length === 0) return <SidebarMessage>No connections.</SidebarMessage>
                return list.map((conn) => (
                  <ConnectionRow
                    key={conn.id}
                    connection={conn}
                    isConnected={connectionActions.connectedIds.has(conn.id)}
                    connIndent={0}
                    envLabel={envFilter === 'all' ? envNameById(conn.environment_id) : undefined}
                    orgSlug={orgSlug}
                    filter={filter}
                    canEditConnection={canEditConnection}
                    canDeleteConnection={canDeleteConnection}
                    onOpen={() => connectionActions.openConnection(conn)}
                    onOpenConsole={() => connectionActions.openConnectionConsole(conn)}
                    onConnect={() => connectionActions.connect(conn)}
                    onDisconnect={() => handleDisconnect(conn)}
                  />
                ))
              })()
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
              <DialogClose
                render={
                  <Button type="button" variant="ghost" disabled={createEnvironment.isPending} />
                }
              >
                Cancel
              </DialogClose>
              <Button type="submit" disabled={createEnvironment.isPending}>
                {createEnvironment.isPending ? 'Creating...' : 'Create'}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog
        open={renamingEnvironment !== null}
        onOpenChange={(open) => {
          if (!open) {
            setRenamingEnvironment(null)
            setRenameName('')
            setRenameNameError(undefined)
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Rename environment</DialogTitle>
          </DialogHeader>
          <form className="mt-6 flex flex-col gap-4" onSubmit={handleRenameSubmit}>
            <FieldGroup>
              <Field data-invalid={renameNameError ? true : undefined}>
                <FieldLabel htmlFor="rename-environment-name">Name</FieldLabel>
                <Input
                  id="rename-environment-name"
                  value={renameName}
                  disabled={renameEnvironment.isPending}
                  aria-invalid={renameNameError ? true : undefined}
                  autoFocus
                  onChange={(event) => {
                    setRenameName(event.target.value)
                    setRenameNameError(undefined)
                  }}
                />
                <FieldError>{renameNameError}</FieldError>
              </Field>
            </FieldGroup>
            <DialogFooter>
              <DialogClose
                render={
                  <Button type="button" variant="ghost" disabled={renameEnvironment.isPending} />
                }
              >
                Cancel
              </DialogClose>
              <Button type="submit" disabled={renameEnvironment.isPending}>
                {renameEnvironment.isPending ? 'Renaming...' : 'Rename'}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <AlertDialog
        open={deletingEnvironment !== null}
        onOpenChange={(open) => {
          if (!open && !deleteEnvironment.isPending) setDeletingEnvironment(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete environment?</AlertDialogTitle>
            <AlertDialogDescription>
              {deletingEnvironmentConnectionCount > 0 ? (
                <>
                  <span className="font-medium text-foreground">{deletingEnvironment?.name}</span>{' '}
                  contains {deletingEnvironmentConnectionCount}{' '}
                  {deletingEnvironmentConnectionCount === 1 ? 'connection' : 'connections'}. Move or
                  delete them before deleting this environment.
                </>
              ) : (
                <>
                  This permanently deletes{' '}
                  <span className="font-medium text-foreground">{deletingEnvironment?.name}</span>.
                </>
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel variant="ghost" disabled={deleteEnvironment.isPending}>
              Cancel
            </AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={deleteEnvironment.isPending || deletingEnvironmentConnectionCount > 0}
              onClick={() => {
                if (deletingEnvironment) {
                  void deleteEnvironment.mutateAsync(deletingEnvironment).catch(() => {})
                }
              }}
            >
              {deleteEnvironment.isPending ? 'Deleting...' : 'Delete'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <ConnectionDialog
        open={addConnOpen || addConnEnvironmentId !== null}
        onOpenChange={(open) => {
          if (!open) {
            setAddConnOpen(false)
            setAddConnEnvironmentId(null)
          }
        }}
        orgSlug={orgSlug}
        workspaceId={workspace.id}
        environments={envItems}
        lockedEnvironmentId={addConnEnvironmentId ?? undefined}
      />

      <TransactionGuardDialog
        open={blockedDisconnectConnection !== null}
        reason="close-connection"
        pendingStatements={blockedDisconnectTransaction.state.pendingStatements}
        onOpenChange={(open) => {
          if (!open) setBlockedDisconnectConnection(null)
        }}
        onCommit={() => {
          void (async () => {
            if (!blockedDisconnectConnection) return
            await blockedDisconnectTransaction.commit()
            connectionActions.disconnect(blockedDisconnectConnection, false)
            setBlockedDisconnectConnection(null)
          })()
        }}
        onRollback={() => {
          void (async () => {
            if (!blockedDisconnectConnection) return
            await blockedDisconnectTransaction.rollback()
            connectionActions.disconnect(blockedDisconnectConnection, false)
            setBlockedDisconnectConnection(null)
          })()
        }}
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
  canEditConnection,
  canDeleteConnection,
  onOpen,
  onOpenConsole,
  onConnect,
  onDisconnect,
  onAddConnection,
  onRenameEnvironment,
  onDeleteEnvironment,
}: {
  environment: Environment
  connections: Connection[]
  connectedIds: Set<number>
  orgSlug: string
  filter: string
  canEditConnection: boolean
  canDeleteConnection: boolean
  onOpen: (conn: Connection) => void
  onOpenConsole: (conn: Connection) => void
  onConnect: (conn: Connection) => void
  onDisconnect: (conn: Connection) => void
  onAddConnection: () => void
  onRenameEnvironment: () => void
  onDeleteEnvironment: () => void
}) {
  const nodeKey = `env:${environment.id}`
  const navigate = useNavigate()
  const stored = useIde((s) => s.expandedNodes[nodeKey])
  const setNodeExpanded = useIde((s) => s.setNodeExpanded)
  const expanded = stored ?? connections.length > 0

  const envPermissions = useQuery(
    orgEffectivePermissionsQueryOptions(orgSlug, 'environment', environment.id),
  )
  const canCreateConnection = hasPermission(envPermissions.data?.permissions, permission.connCreate)
  const canRenameEnvironment = hasPermission(envPermissions.data?.permissions, permission.envWrite)
  const canDeleteEnvironment = hasPermission(envPermissions.data?.permissions, permission.envDelete)

  return (
    <div>
      <ContextMenu
        items={buildEnvironmentMenu({
          onCopyName: () => copyWithToast(environment.name),
          onManageEnvironments: () =>
            navigate({
              to: '/orgs/$org_slug/workspaces/$workspace_id/environments',
              params: { org_slug: orgSlug, workspace_id: String(environment.workspace_id) },
            }),
          onNewConnection: canCreateConnection ? onAddConnection : undefined,
          onRenameEnvironment: canRenameEnvironment ? onRenameEnvironment : undefined,
          onDeleteEnvironment: canDeleteEnvironment ? onDeleteEnvironment : undefined,
        })}
      >
        <div className="group mx-1 flex h-6 items-center rounded-md text-xs transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground">
          <button
            type="button"
            onClick={() => setNodeExpanded(nodeKey, !expanded)}
            className="flex min-w-0 flex-1 items-center gap-1.5 px-2 text-left"
          >
            <Icon
              name={expanded ? 'chevron-down' : 'chevron-right'}
              size={11}
              className="shrink-0 text-muted-foreground"
            />
            <Icon name="box" size={13} className="shrink-0 text-muted-foreground" />
            <span className="min-w-0 flex-1 truncate font-medium" title={environment.name}>
              {environment.name}
            </span>
          </button>
          {canCreateConnection && (
            <Tip label={`New connection in ${environment.name}`}>
              <button
                type="button"
                onClick={onAddConnection}
                aria-label={`New connection in ${environment.name}`}
                className="mr-1 flex size-5 shrink-0 items-center justify-center rounded opacity-0 transition-opacity hover:bg-muted group-hover:opacity-100"
              >
                <Icon name="plus-sign" size={11} />
              </button>
            </Tip>
          )}
        </div>
      </ContextMenu>

      {expanded && (
        <div>
          {connections.length === 0 ? (
            <div className="py-1.5 pl-[18px] pr-2 text-xs text-muted-foreground">
              No connections.
            </div>
          ) : (
            connections.map((conn) => (
              <ConnectionRow
                key={conn.id}
                connection={conn}
                isConnected={connectedIds.has(conn.id)}
                connIndent={18}
                orgSlug={orgSlug}
                filter={filter}
                canEditConnection={canEditConnection}
                canDeleteConnection={canDeleteConnection}
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
  connIndent,
  envLabel,
  orgSlug,
  filter,
  canEditConnection,
  canDeleteConnection,
  onOpen,
  onOpenConsole,
  onConnect,
  onDisconnect,
}: {
  connection: Connection
  isConnected: boolean
  connIndent: number
  envLabel?: string
  orgSlug: string
  filter: string
  canEditConnection: boolean
  canDeleteConnection: boolean
  onOpen: () => void
  onOpenConsole: () => void
  onConnect: () => void
  onDisconnect: () => void
}) {
  const nodeKey = `conn:${connection.id}`
  const navigate = useNavigate()
  const storedExpanded = useIde((s) => s.expandedNodes[nodeKey])
  const setNodeExpanded = useIde((s) => s.setNodeExpanded)
  const expanded = storedExpanded ?? false
  const sessionId = useIde((s) => s.sessions[connection.id])
  const connStatus = useIde((s) => s.connectionStatus[connection.id])
  const connState = resolveConnectionState(Boolean(sessionId), connStatus)
  // Hint the connection used by the active tab (file's linked connection, console, or connection tab).
  const isActive = useIde((s) => {
    const id = selectActiveTabId(s, connection.workspace_id)
    return s.tabs.find((t) => t.id === id)?.connectionId === connection.id
  })
  const queryClient = useQueryClient()
  const [editOpen, setEditOpen] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const refresh = useSchemaRefresh({
    orgSlug,
    workspaceId: connection.workspace_id,
    connectionId: connection.id,
    sessionId,
  })
  const deleteConnection = useMutation({
    mutationFn: () =>
      api.delete<void>(
        `/api/v1/orgs/${orgSlug}/workspaces/${connection.workspace_id}/connections/${connection.id}`,
      ),
    onSuccess: async () => {
      setDeleteOpen(false)
      toast.success('Connection deleted')
      await queryClient.invalidateQueries({
        queryKey: queryKeys.orgWorkspaceConnectionsScope(orgSlug, connection.workspace_id),
      })
    },
    onError: (error) => {
      toast.error(errorMessage(error, 'Failed to delete connection'))
    },
  })

  const menuItems = buildConnectionMenu({
    isConnected,
    canEditConnection,
    canDeleteConnection,
    onOpen,
    onOpenConsole,
    onConnect,
    onDisconnect,
    onRefreshSchema: () => refresh.mutate(),
    onCopyName: () => copyWithToast(connection.name),
    onManageConnections: () =>
      navigate({
        to: '/orgs/$org_slug/workspaces/$workspace_id/connections',
        params: { org_slug: orgSlug, workspace_id: String(connection.workspace_id) },
      }),
    onEditConnection: () => setEditOpen(true),
    onDeleteConnection: () => setDeleteOpen(true),
  })

  return (
    <div>
      <ContextMenu items={menuItems}>
        <div
          style={{ paddingLeft: connIndent }}
          className={cn(
            'flex items-center transition-colors',
            sidebarActiveRowClass(isActive),
          )}
        >
          {isConnected || expanded ? (
            <button
              type="button"
              aria-label={expanded ? 'Collapse schema' : 'Expand schema'}
              onClick={() => setNodeExpanded(nodeKey, !expanded)}
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
            <span className="truncate" title={connection.name}>
              {connection.name}
            </span>
          </button>

          {isConnected && (
            <Tip label="Refresh schema">
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
                <Icon
                  name="refresh"
                  size={11}
                  className={refresh.isPending ? 'animate-spin' : undefined}
                />
              </button>
            </Tip>
          )}
          <div className="flex h-6 min-w-0 flex-1 items-center justify-end">
            {envLabel && (
              <span
                className="min-w-0 truncate pr-1 text-[10px] text-muted-foreground"
                title={envLabel}
              >
                {envLabel}
              </span>
            )}
          </div>
        </div>
      </ContextMenu>

      {/* Stays mounted while disconnected so an expanded tree can show its
          "Not connected · Connect" hint instead of vanishing when the
          server-side session expires. */}
      {expanded && (
        <div style={{ marginLeft: connIndent + 14 }} className="border-l border-border/60">
          <SchemaTree
            orgSlug={orgSlug}
            workspaceId={connection.workspace_id}
            connectionId={connection.id}
            driver={connection.driver}
            filter={filter}
            onConnect={onConnect}
          />
        </div>
      )}

      <EditConnectionDialog
        open={editOpen}
        onOpenChange={setEditOpen}
        orgSlug={orgSlug}
        workspaceId={connection.workspace_id}
        connection={connection}
        canRevealDsn={canEditConnection}
      />

      <AlertDialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete connection?</AlertDialogTitle>
            <AlertDialogDescription>
              This permanently deletes{' '}
              <span className="font-medium text-foreground">{connection.name}</span> and drops any
              active sessions.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel variant="ghost" disabled={deleteConnection.isPending}>
              Cancel
            </AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={deleteConnection.isPending}
              onClick={() => deleteConnection.mutate()}
            >
              {deleteConnection.isPending ? 'Deleting...' : 'Delete'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
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
      className={cn(
        'absolute -bottom-0.5 -right-0.5 size-1.5 rounded-full ring-1 ring-sidebar',
        color,
      )}
    />
  )
}

function SidebarMessage({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex items-center gap-2 px-3 py-3 text-xs text-muted-foreground">
      {children}
    </div>
  )
}
