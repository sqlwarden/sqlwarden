import { useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Icon } from '#/lib/icons'
import { cn } from '#/lib/utils'
import { api } from '#/lib/api/client'
import type { ObjectRef, Workspace } from '#/lib/api/types'
import {
  orgConnectionObjectQueryOptions,
  orgConnectionSchemaSpecQueryOptions,
  refreshConnectionSchema,
  connectionObjectQueryKey,
} from '#/lib/api/query'
import { dialectFor } from '../sqlDialect'
import { useIde, type EditorTab } from '../useIdeStore'
import { getObjectRenderer, type HeaderBadge, type ObjectViewModel } from './registry'
import { resolveObjectViewState, type ObjectViewState } from './viewState'

export function ObjectDetailView({ orgSlug, workspace, tab }: { orgSlug: string; workspace: Workspace; tab: EditorTab }) {
  const ref = tab.objectRef
  const connectionId = tab.connectionId
  const driver = tab.driver ?? 'postgres'
  const sessionId = useIde((s) => (connectionId ? s.sessions[connectionId] : undefined))
  const setSession = useIde((s) => s.setSession)
  const setConnectionStatus = useIde((s) => s.setConnectionStatus)
  const queryClient = useQueryClient()
  const [activeSection, setActiveSection] = useState<string>('columns')

  const detailQuery = useQuery({
    ...orgConnectionObjectQueryOptions(orgSlug, workspace.id, connectionId ?? 0, sessionId ?? '', ref ?? EMPTY_REF),
    enabled: Boolean(sessionId && connectionId && ref),
  })
  const specQuery = useQuery({
    ...orgConnectionSchemaSpecQueryOptions(orgSlug, workspace.id, connectionId ?? 0, sessionId ?? ''),
    enabled: Boolean(sessionId && connectionId),
  })

  const detail = detailQuery.data ?? null
  const state = resolveObjectViewState({
    hasSession: Boolean(sessionId),
    isLoading: detailQuery.isLoading,
    error: detailQuery.error,
    hasData: Boolean(detail),
  })

  const renderer = getObjectRenderer(driver)
  const vm: ObjectViewModel | null =
    detail && connectionId
      ? { detail, spec: specQuery.data?.spec, dialect: dialectFor(driver), driver, orgSlug, workspaceId: workspace.id, connectionId, sessionId: sessionId ?? '' }
      : null
  const sections = useMemo(() => (vm ? renderer.sections(vm) : []), [vm, renderer])
  const current = sections.find((s) => s.id === activeSection) ?? sections[0]

  if (!ref || !connectionId) {
    return <StatePane state={{ kind: 'error', message: 'This tab is missing its object reference.' }} driver={driver} onReconnect={noop} />
  }

  async function reconnect() {
    setConnectionStatus(connectionId!, 'connecting')
    try {
      const data = await api.post<{ session_id: string }>(
        `/api/v1/orgs/${orgSlug}/workspaces/${workspace.id}/connections/${connectionId}/connect`,
      )
      setSession(connectionId!, data.session_id)
    } catch {
      /* the next query attempt surfaces the failure */
    } finally {
      setConnectionStatus(connectionId!, null)
    }
  }

  async function refresh() {
    if (!sessionId || !ref || !connectionId) return
    await refreshConnectionSchema(orgSlug, workspace.id, connectionId, sessionId, ref)
    await queryClient.invalidateQueries({ queryKey: connectionObjectQueryKey(orgSlug, workspace.id, connectionId, ref) })
  }

  return (
    <div className="flex h-full min-h-0 flex-col bg-card">
      <Header
        objectRef={ref}
        driver={driver}
        badges={vm ? renderer.headerBadges(vm) : []}
        onRefresh={refresh}
        canRefresh={state.kind === 'ready'}
      />
      <div className="min-h-0 flex-1">
        {state.kind === 'ready' && vm && current ? (
          <div className="flex h-full min-h-0">
            <nav className="w-40 shrink-0 overflow-auto border-r border-border py-1">
              {sections.map((s) => (
                <button
                  key={s.id}
                  type="button"
                  onClick={() => setActiveSection(s.id)}
                  className={cn(
                    'flex w-full items-center gap-2 px-3 py-1.5 text-left text-xs transition-colors',
                    s.id === current.id ? 'bg-accent text-accent-foreground' : 'text-muted-foreground hover:bg-accent/40',
                  )}
                >
                  <Icon name={s.icon} size={13} className="shrink-0" />
                  {s.label}
                </button>
              ))}
            </nav>
            <div className="min-h-0 flex-1 overflow-auto">{current.render(vm)}</div>
          </div>
        ) : (
          <StatePane state={state} objectRef={ref} driver={driver} onReconnect={reconnect} />
        )}
      </div>
    </div>
  )
}

const EMPTY_REF: ObjectRef = { namespace: '', kind: '', name: '' }
function noop() {}

function Header({
  objectRef,
  driver,
  badges,
  onRefresh,
  canRefresh,
}: {
  objectRef: ObjectRef
  driver: string
  badges: HeaderBadge[]
  onRefresh: () => void
  canRefresh: boolean
}) {
  return (
    <div className="flex h-10 shrink-0 items-center gap-2 border-b border-border px-3">
      <span className="truncate text-sm font-medium text-foreground">
        {objectRef.namespace ? `${objectRef.namespace}.` : ''}
        {objectRef.name}
      </span>
      <Tag>{objectRef.kind}</Tag>
      <span className="text-xs text-muted-foreground">{driver}</span>
      {badges.map((b) => (
        <Tag key={b.id}>
          {b.label}: {b.value}
        </Tag>
      ))}
      <div className="flex-1" />
      <button
        type="button"
        onClick={onRefresh}
        disabled={!canRefresh}
        aria-label="Refresh"
        className="rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-40"
      >
        <Icon name="refresh" size={14} />
      </button>
    </div>
  )
}

function Tag({ children }: { children: React.ReactNode }) {
  return <span className="rounded bg-muted px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground">{children}</span>
}

function StatePane({
  state,
  objectRef,
  driver,
  onReconnect,
}: {
  state: ObjectViewState
  objectRef?: ObjectRef
  driver: string
  onReconnect: () => void
}) {
  if (state.kind === 'loading') {
    return (
      <Center>
        <Icon name="loading-03" size={16} className="animate-spin" />
        Loading object…
      </Center>
    )
  }
  if (state.kind === 'unsupported') {
    return <Center>This driver doesn&apos;t support object details.</Center>
  }
  if (state.kind === 'forbidden') {
    return <Center className="text-destructive">You no longer have access to this connection.</Center>
  }
  if (state.kind === 'error') {
    return <Center className="text-destructive">{state.message}</Center>
  }
  // no-session
  return (
    <div className="flex h-full flex-col items-center justify-center gap-3 text-center">
      <div className="flex flex-col gap-1">
        <div className="text-sm font-medium text-foreground">
          {objectRef?.namespace ? `${objectRef.namespace}.` : ''}
          {objectRef?.name}
        </div>
        <div className="text-xs text-muted-foreground">{driver} · connection not available</div>
      </div>
      <button
        type="button"
        onClick={onReconnect}
        className="rounded border border-border px-3 py-1.5 text-xs text-foreground hover:bg-accent"
      >
        Reconnect
      </button>
    </div>
  )
}

function Center({ children, className = '' }: { children: React.ReactNode; className?: string }) {
  return <div className={`flex h-full items-center justify-center gap-2 text-xs text-muted-foreground ${className}`}>{children}</div>
}
