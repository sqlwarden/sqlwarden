import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { HugeiconsIcon } from '@hugeicons/react'
import {
  ArrowDown01Icon,
  ArrowRight01Icon,
  DatabaseIcon,
  ServerStack01Icon,
} from '@hugeicons/core-free-icons'
import {
  orgEnvironmentsQueryOptions,
  orgWorkspaceConnectionsQueryOptions,
} from '#/lib/api/query'
import type { Connection, Environment, Workspace } from '#/lib/api/types'
import { cn } from '#/lib/utils'
import { useIde, newConnectionTab } from './useIdeStore'
import { SidebarPane } from './SidebarPane'

type DatabasePanelProps = {
  orgSlug: string
  workspace: Workspace
  maximized: boolean
  onMaximizedChange: (maximized: boolean) => void
}

export function DatabasePanel({ orgSlug, workspace, maximized, onMaximizedChange }: DatabasePanelProps) {
  const openTab = useIde((s) => s.openTab)

  const environments = useQuery(
    orgEnvironmentsQueryOptions(orgSlug, workspace.id, { page_size: 100, sort: 'name', order: 'asc' }),
  )
  const connections = useQuery(
    orgWorkspaceConnectionsQueryOptions(orgSlug, workspace.id, { page_size: 100, sort: 'name', order: 'asc' }),
  )

  const envItems = environments.data?.items ?? []
  const connItems = connections.data?.items ?? []

  return (
    <SidebarPane
      title={workspace.name}
      icon={DatabaseIcon}
      maximized={maximized}
      onMaximizedChange={onMaximizedChange}
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
