import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Icon } from '#/lib/icons'
import { cn } from '#/lib/utils'
import { api } from '#/lib/api/client'
import type { Connection, ObjectRef, Workspace } from '#/lib/api/types'
import { scopeLabel } from '#/lib/api/scope'
import {
  orgConnectionObjectQueryOptions,
  orgConnectionSchemaSpecQueryOptions,
} from '#/lib/api/query'
import { dialectFor } from '../sqlDialect'
import { useIde, type EditorTab } from '../useIdeStore'
import { newDiagramTab } from '../schema-diagram/diagramTab'
import { diagramSupportedForKind } from '../schema-diagram/capability'
import { getObjectRenderer, type HeaderBadge, type ObjectViewModel } from './registry'
import { resolveObjectViewState, type ObjectViewState } from './viewState'
import { Tip } from '../schema-diagram/Tip'
import { useEvictGoneSession } from '../sessionErrors'
import { useSchemaRefresh } from '../useSchemaRefresh'

export function ObjectDetailView({
  orgSlug,
  workspace,
  tab,
}: {
  orgSlug: string
  workspace: Workspace
  tab: EditorTab
}) {
  const ref = tab.objectRef
  const connectionId = tab.connectionId
  const driver = tab.driver ?? 'postgres'
  const sessionId = useIde((s) => (connectionId ? s.sessions[connectionId] : undefined))
  const setSession = useIde((s) => s.setSession)
  const setConnectionStatus = useIde((s) => s.setConnectionStatus)
  const openTab = useIde((s) => s.openTab)
  const [activeSection, setActiveSection] = useState<string>('columns')

  const refreshSchema = useSchemaRefresh({
    orgSlug,
    workspaceId: workspace.id,
    connectionId: connectionId ?? 0,
    sessionId,
    ref,
  })

  const detailQuery = useQuery({
    ...orgConnectionObjectQueryOptions(
      orgSlug,
      workspace.id,
      connectionId ?? 0,
      sessionId,
      ref ?? EMPTY_REF,
    ),
    enabled: Boolean(connectionId && ref),
  })
  const specQuery = useQuery({
    ...orgConnectionSchemaSpecQueryOptions(orgSlug, workspace.id, connectionId ?? 0, sessionId),
    enabled: Boolean(connectionId && ref),
  })

  // A 410 means the server-side session died while this tab was open — drop it
  // so the view flips to the reconnect pane instead of erroring on stale data.
  useEvictGoneSession(connectionId, [detailQuery.error, specQuery.error])

  const detail = detailQuery.data ?? null
  const state = resolveObjectViewState({
    hasSession: Boolean(sessionId) || Boolean(detail),
    isLoading: detailQuery.isLoading,
    error: detailQuery.error,
    hasData: Boolean(detail),
  })

  const renderer = getObjectRenderer(driver)
  const vm = useMemo<ObjectViewModel | null>(
    () =>
      detail && connectionId
        ? {
            detail,
            spec: specQuery.data?.spec,
            dialect: dialectFor(driver),
            driver,
            orgSlug,
            workspaceId: workspace.id,
            connectionId,
            sessionId: sessionId ?? '',
          }
        : null,
    [connectionId, detail, driver, orgSlug, sessionId, specQuery.data?.spec, workspace.id],
  )
  const sections = useMemo(() => (vm ? renderer.sections(vm) : []), [vm, renderer])
  const current = sections.find((s) => s.id === activeSection) ?? sections[0]

  if (!ref || !connectionId) {
    return (
      <StatePane
        state={{ kind: 'error', message: 'This tab is missing its object reference.' }}
        driver={driver}
        onReconnect={noop}
      />
    )
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

  return (
    <div className="flex h-full min-h-0 flex-col bg-card">
      <Header
        objectRef={ref}
        driver={driver}
        badges={vm ? renderer.headerBadges(vm) : []}
        onRefresh={() => refreshSchema.mutate()}
        canRefresh={state.kind === 'ready'}
        refreshing={refreshSchema.isPending}
        onViewInDiagram={
          ref && connectionId && diagramSupportedForKind(specQuery.data?.spec, ref.kind)
            ? () =>
                openTab(
                  newDiagramTab({ id: connectionId, driver } as Connection, workspace, {
                    kind: 'object',
                    ref,
                  }),
                )
            : undefined
        }
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
                    s.id === current.id
                      ? 'bg-accent text-accent-foreground'
                      : 'text-muted-foreground hover:bg-accent/40',
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

const EMPTY_REF: ObjectRef = { scope: [], kind: '', name: '' }
function noop() {}

function Header({
  objectRef,
  driver,
  badges,
  onRefresh,
  canRefresh,
  refreshing,
  onViewInDiagram,
}: {
  objectRef: ObjectRef
  driver: string
  badges: HeaderBadge[]
  onRefresh: () => void
  canRefresh: boolean
  refreshing: boolean
  onViewInDiagram?: () => void
}) {
  return (
    <div className="flex h-10 shrink-0 items-center gap-2 border-b border-border px-3">
      <span className="truncate text-sm font-medium text-foreground">
        {objectRef.scope.length > 0 ? `${scopeLabel(objectRef.scope)}.` : ''}
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
      {onViewInDiagram && (
        <Tip label="View in diagram">
          <button
            type="button"
            onClick={onViewInDiagram}
            aria-label="View in diagram"
            className="rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
          >
            <Icon name="flow-connection" size={14} />
          </button>
        </Tip>
      )}
      <Tip
        label={
          refreshing
            ? 'Refreshing schema'
            : canRefresh
              ? `Refresh ${objectRef.kind || 'object'}`
              : 'Connect first to refresh'
        }
      >
        {/* Span wrapper so the tooltip still fires while the button is disabled
            (disabled buttons swallow pointer events). */}
        <span className={cn('inline-flex', !canRefresh && 'cursor-not-allowed')}>
          <button
            type="button"
            onClick={onRefresh}
            disabled={!canRefresh || refreshing}
            aria-label="Refresh"
            className="rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-40"
          >
            <Icon name="refresh" size={14} className={refreshing ? 'animate-spin' : undefined} />
          </button>
        </span>
      </Tip>
    </div>
  )
}

function Tag({ children }: { children: React.ReactNode }) {
  return (
    <span className="rounded bg-muted px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground">
      {children}
    </span>
  )
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
    return (
      <Center className="text-destructive">You no longer have access to this connection.</Center>
    )
  }
  if (state.kind === 'error') {
    return <Center className="text-destructive">{state.message}</Center>
  }
  // no-session
  return (
    <div className="flex h-full flex-col items-center justify-center gap-3 text-center">
      <div className="flex flex-col gap-1">
        <div className="text-sm font-medium text-foreground">
          {objectRef?.scope?.length ? `${scopeLabel(objectRef.scope)}.` : ''}
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
  return (
    <div
      className={`flex h-full items-center justify-center gap-2 text-xs text-muted-foreground ${className}`}
    >
      {children}
    </div>
  )
}
