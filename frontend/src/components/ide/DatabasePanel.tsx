import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { HugeiconsIcon } from '@hugeicons/react'
import {
  ArrowDown01Icon,
  ArrowRight01Icon,
  DatabaseIcon,
  FlowConnectionIcon,
  PlusSignIcon,
  ServerStack01Icon,
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
import { useIde, newConnectionTab } from './useIdeStore'
import { SidebarPane } from './SidebarPane'
import { ConnectionDialog } from './ConnectionDialog'
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
  const queryClient = useQueryClient()

  const [addEnvOpen, setAddEnvOpen] = useState(false)
  const [addConnOpen, setAddConnOpen] = useState(false)
  const [envName, setEnvName] = useState('')
  const [envDescription, setEnvDescription] = useState('')
  const [envNameError, setEnvNameError] = useState<string | undefined>(undefined)

  const effectivePermissions = useQuery(
    orgEffectivePermissionsQueryOptions(orgSlug, 'workspace', workspace.id),
  )
  const canCreateEnvironment = hasPermission(effectivePermissions.data?.permissions, permission.envCreate)
  const canCreateConnection = hasPermission(effectivePermissions.data?.permissions, permission.connCreate)

  const environments = useQuery(
    orgEnvironmentsQueryOptions(orgSlug, workspace.id, { page_size: 100, sort: 'name', order: 'asc' }),
  )
  const connections = useQuery(
    orgWorkspaceConnectionsQueryOptions(orgSlug, workspace.id, { page_size: 100, sort: 'name', order: 'asc' }),
  )

  const envItems = environments.data?.items ?? []
  const connItems = connections.data?.items ?? []

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

  const actions = (canCreateEnvironment || canCreateConnection) ? (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label="Add to Explorer"
          />
        }
      >
        <HugeiconsIcon icon={PlusSignIcon} size={14} strokeWidth={2} />
      </DropdownMenuTrigger>
      <DropdownMenuContent side="bottom" align="end" className="min-w-44">
        {canCreateEnvironment ? (
          <DropdownMenuItem onClick={() => setAddEnvOpen(true)}>
            <HugeiconsIcon icon={ServerStack01Icon} size={14} strokeWidth={2} className="text-muted-foreground" />
            New Environment
          </DropdownMenuItem>
        ) : null}
        {canCreateConnection ? (
          <DropdownMenuItem onClick={() => setAddConnOpen(true)}>
            <HugeiconsIcon icon={FlowConnectionIcon} size={14} strokeWidth={2} className="text-muted-foreground" />
            New Connection
          </DropdownMenuItem>
        ) : null}
      </DropdownMenuContent>
    </DropdownMenu>
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
              onOpenTab={(conn) => openTab(newConnectionTab(conn, workspace))}
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
        open={addConnOpen}
        onOpenChange={setAddConnOpen}
        orgSlug={orgSlug}
        workspaceId={workspace.id}
        environments={envItems}
      />
    </>
  )
}

function EnvironmentRow({
  environment,
  connections,
  onOpenTab,
}: {
  environment: Environment
  connections: Connection[]
  onOpenTab: (conn: Connection) => void
}) {
  const [expanded, setExpanded] = useState(true)

  return (
    <div>
      <button
        type="button"
        onClick={() => setExpanded((v) => !v)}
        className="flex h-7 w-full items-center gap-1.5 px-2 text-left text-xs transition-colors hover:bg-accent hover:text-accent-foreground"
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

      {expanded && (
        <div className="border-l border-border ml-[18px]">
          {connections.length === 0 ? (
            <div className="px-3 py-1.5 text-xs text-muted-foreground">No connections.</div>
          ) : (
            connections.map((conn) => (
              <ConnectionRow key={conn.id} connection={conn} onOpenTab={onOpenTab} />
            ))
          )}
        </div>
      )}
    </div>
  )
}

function ConnectionRow({
  connection,
  onOpenTab,
}: {
  connection: Connection
  onOpenTab: (conn: Connection) => void
}) {
  return (
    <button
      type="button"
      onClick={() => onOpenTab(connection)}
      className={cn(
        'flex h-7 w-full items-center gap-2 px-3 text-left text-xs',
        'transition-colors hover:bg-accent hover:text-accent-foreground',
      )}
    >
      <HugeiconsIcon icon={DatabaseIcon} size={13} strokeWidth={2} className="shrink-0 text-muted-foreground" />
      <span className="min-w-0 flex-1 truncate">{connection.name}</span>
    </button>
  )
}

function SidebarMessage({ children }: { children: React.ReactNode }) {
  return <div className="px-3 py-3 text-xs text-muted-foreground">{children}</div>
}
