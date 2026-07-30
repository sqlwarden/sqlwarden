import { createContext, useContext, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Icon, type AppIcon } from '#/lib/icons'
import { cn } from '#/lib/utils'
import { isApiError } from '#/lib/api/errors'
import {
  orgConnectionCatalogQueryOptions,
  orgConnectionSchemaSpecQueryOptions,
  orgConnectionObjectQueryOptions,
} from '#/lib/api/query'
import type {
  CatalogNamespace,
  CatalogObjectGroup,
  Connection,
  ObjectDescriptor,
  ObjectDetail,
  ObjectRef,
  SchemaSpec,
  Workspace,
} from '#/lib/api/types'
import { useIde } from './useIdeStore'
import { newObjectTab } from './object-detail/objectTab'
import { newDiagramTab, type DiagramTarget } from './schema-diagram/diagramTab'
import { diagramSupported, diagramSupportedForKind } from './schema-diagram/capability'
import { OBJECT_REF_DND_MIME } from './schema-diagram/dnd'
import { filterCatalog, kindLabel, sortedGroups } from './schemaCatalog'
import { columnTypeIcon, columnTypeIconColor } from './columnTypeIcon'
import { dialectFor, IDENTIFIER_DND_MIME, type SqlDialect } from './sqlDialect'
import { useInsertIntoEditor } from './useInsertIntoEditor'
import { ContextMenu } from '#/components/ui/context-menu'
import { copyWithToast, columnList, qualifiedColumn } from './contextMenus/clipboard'
import { buildNamespaceMenu, buildObjectGroupMenu } from './contextMenus/schemaMenu'
import { buildObjectMenu } from './contextMenus/objectMenu'
import { buildColumnMenu, buildIndexMenu } from './contextMenus/columnMenu'
import { useEvictGoneSession } from './sessionErrors'
import {
  invalidateConnectionSchemaQueries,
  refreshConnectionSchema,
} from '#/lib/api/queries/database'

type TreeCtx = {
  dialect: SqlDialect
  insert: (text: string) => void
  refresh: () => void
  openObject: (ref: ObjectRef) => void
  openDiagram: (target: DiagramTarget) => void
  spec: SchemaSpec | undefined
  orgSlug: string
  workspaceId: number
  connectionId: number
  sessionId?: string
}

const SchemaTreeContext = createContext<TreeCtx | null>(null)

type InsertableProps = {
  draggable: boolean
  onDragStart: (e: React.DragEvent) => void
  onDoubleClick: () => void
}

function dragPropsFor(
  ctx: { insert: (text: string) => void } | null,
  text: string | undefined,
  objectRef?: ObjectRef,
): InsertableProps | undefined {
  if (!ctx || text === undefined) return undefined
  return {
    draggable: true,
    onDragStart: (e) => {
      e.dataTransfer.setData(IDENTIFIER_DND_MIME, text)
      e.dataTransfer.setData('text/plain', text)
      // Object rows also carry their ref so they can be dropped onto a diagram.
      if (objectRef) e.dataTransfer.setData(OBJECT_REF_DND_MIME, JSON.stringify(objectRef))
      e.dataTransfer.effectAllowed = 'copy'
      e.stopPropagation()
    },
    onDoubleClick: () => ctx.insert(text),
  }
}

function useObjectInsert(ref: ObjectRef): InsertableProps | undefined {
  const ctx = useContext(SchemaTreeContext)
  return dragPropsFor(ctx, ctx ? ctx.dialect.formatObject(ref.namespace, ref.name) : undefined, ref)
}

function useColumnInsert(name: string): InsertableProps | undefined {
  const ctx = useContext(SchemaTreeContext)
  return dragPropsFor(ctx, ctx ? ctx.dialect.formatColumn(name) : undefined)
}

function useTreeCtx() {
  const ctx = useContext(SchemaTreeContext)
  return {
    dialect: ctx?.dialect ?? null,
    refresh: ctx?.refresh ?? (() => {}),
    spec: ctx?.spec,
    openDiagram: ctx?.openDiagram,
  }
}

/** Icon + accent color per object kind, so tables/views/functions scan at a glance. */
const KIND_STYLE: Record<string, { icon: AppIcon; className: string }> = {
  table: { icon: 'table', className: 'text-chart-4' },
  view: { icon: 'eye', className: 'text-chart-2' },
  materialized_view: { icon: 'eye', className: 'text-chart-2' },
  function: { icon: 'play', className: 'text-chart-1' },
  procedure: { icon: 'terminal', className: 'text-chart-1' },
  sequence: { icon: 'sort', className: 'text-chart-3' },
  trigger: { icon: 'flow-connection', className: 'text-chart-5' },
}

const kindStyle = (kind: string) =>
  KIND_STYLE[kind] ?? { icon: 'box' as AppIcon, className: 'text-muted-foreground' }

export function SchemaTree({
  orgSlug,
  workspaceId,
  connectionId,
  driver,
  filter,
  onConnect,
}: {
  orgSlug: string
  workspaceId: number
  connectionId: number
  driver: string
  filter: string
  /** Starts a fresh session for this connection (used by the reconnect hint). */
  onConnect?: () => void
}) {
  const sessionId = useIde((s) => s.sessions[connectionId])
  const connStatus = useIde((s) => s.connectionStatus[connectionId])
  const openTab = useIde((s) => s.openTab)
  const insert = useInsertIntoEditor()
  const dialect = dialectFor(driver)

  const openObject = (ref: ObjectRef) =>
    openTab(
      newObjectTab(
        { id: connectionId, driver } as Connection,
        { id: workspaceId } as Workspace,
        ref,
      ),
    )

  const openDiagram = (target: DiagramTarget) =>
    openTab(
      newDiagramTab(
        { id: connectionId, driver } as Connection,
        { id: workspaceId } as Workspace,
        target,
      ),
    )

  const catalogQuery = useQuery({
    ...orgConnectionCatalogQueryOptions(orgSlug, workspaceId, connectionId, sessionId),
  })
  const specQuery = useQuery({
    ...orgConnectionSchemaSpecQueryOptions(orgSlug, workspaceId, connectionId, sessionId),
  })
  const queryClient = useQueryClient()
  const refreshSchema = useMutation({
    mutationFn: () => refreshConnectionSchema(orgSlug, workspaceId, connectionId, sessionId ?? ''),
    onSuccess: () =>
      invalidateConnectionSchemaQueries(queryClient, orgSlug, workspaceId, connectionId),
  })

  // A 410 from any schema endpoint means the server-side session died (idle
  // timeout, restart). Drop it so the tree flips to the reconnect hint instead
  // of erroring forever against a dead session id.
  useEvictGoneSession(connectionId, [catalogQuery.error, specQuery.error])

  if (catalogQuery.isLoading) {
    return (
      <SchemaMessage>
        <SchemaSpinner />
        Loading schema...
      </SchemaMessage>
    )
  }
  if (catalogQuery.isError) {
    if (!sessionId) {
      if (connStatus === 'connecting') {
        return (
          <SchemaMessage>
            <SchemaSpinner />
            Connecting…
          </SchemaMessage>
        )
      }
      return (
        <SchemaMessage>
          <span>Not connected.</span>
          {onConnect && (
            <button
              type="button"
              className="font-medium text-primary hover:underline"
              onClick={onConnect}
            >
              Connect
            </button>
          )}
        </SchemaMessage>
      )
    }
    if (isApiError(catalogQuery.error) && catalogQuery.error.status === 501) {
      return <SchemaMessage>This driver doesn&apos;t support schema inspection.</SchemaMessage>
    }
    return (
      <div className="flex items-center gap-2 px-2 py-1.5 text-xs text-muted-foreground">
        <span>Failed to load schema.</span>
        <button
          type="button"
          className="underline hover:text-foreground"
          onClick={() => catalogQuery.refetch()}
        >
          Retry
        </button>
      </div>
    )
  }

  const raw = catalogQuery.data?.catalog
  if (!raw) {
    if (catalogQuery.data?.status === 'pending') {
      return (
        <SchemaMessage>
          <SchemaSpinner />
          Preparing schema snapshot…
        </SchemaMessage>
      )
    }
    return <SchemaMessage>No schema.</SchemaMessage>
  }

  const filtering = filter.trim() !== ''
  const namespaces = filterCatalog(raw, filter).namespaces ?? []

  if (namespaces.length === 0) {
    return <SchemaMessage>{filtering ? 'No matches.' : 'No objects.'}</SchemaMessage>
  }

  const spec = specQuery.data?.spec
  const single = namespaces.length === 1 ? namespaces[0] : null
  const ctx: TreeCtx = {
    dialect,
    insert,
    refresh: () => refreshSchema.mutate(),
    openObject,
    openDiagram,
    spec,
    orgSlug,
    workspaceId,
    connectionId,
    sessionId,
  }

  return (
    <SchemaTreeContext.Provider value={ctx}>
      <div className="py-0.5">
        {single
          ? sortedGroups(single, spec).map((g) => (
              <SchemaGroupNode key={g.kind} group={g} forceOpen={filtering} />
            ))
          : namespaces.map((ns) => (
              <SchemaNamespaceNode key={ns.name} namespace={ns} forceOpen={filtering} />
            ))}
      </div>
    </SchemaTreeContext.Provider>
  )
}

function SchemaMessage({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex items-center gap-1.5 px-2 py-1.5 text-xs text-muted-foreground">
      {children}
    </div>
  )
}

function SchemaSpinner({ size = 12 }: { size?: number }) {
  return (
    <Icon name="loading-03" size={size} className="shrink-0 animate-spin text-muted-foreground" />
  )
}

/** Indent guide: children of an expanded node nest inside this container, which
 *  draws a continuous vertical line under the parent's chevron. */
function GuideChildren({ children }: { children: React.ReactNode }) {
  return <div className="ml-[13px] border-l border-border/60">{children}</div>
}

function SchemaNamespaceNode({
  namespace,
  forceOpen,
}: {
  namespace: CatalogNamespace
  forceOpen: boolean
}) {
  const [open, setOpen] = useState<boolean | null>(null)
  const expanded = open ?? forceOpen
  const { refresh, spec, openDiagram } = useTreeCtx()
  const groups = sortedGroups(namespace, spec)
  const menuItems = buildNamespaceMenu({
    onCopyName: () => copyWithToast(namespace.name),
    onRefresh: refresh,
    onViewDiagram:
      diagramSupported(spec) && openDiagram
        ? () => openDiagram({ kind: 'namespace', namespace: namespace.name })
        : undefined,
  })

  return (
    <div>
      <ContextMenu items={menuItems}>
        <TreeRow
          typeIcon="database"
          chevron={expanded}
          bold
          label={namespace.name}
          onClick={() => setOpen(!expanded)}
        />
      </ContextMenu>
      {expanded && (
        <GuideChildren>
          {groups.map((g) => (
            <SchemaGroupNode key={g.kind} group={g} forceOpen={forceOpen} />
          ))}
        </GuideChildren>
      )}
    </div>
  )
}

function SchemaGroupNode({ group, forceOpen }: { group: CatalogObjectGroup; forceOpen: boolean }) {
  const [open, setOpen] = useState<boolean | null>(null)
  const expanded = open ?? forceOpen
  const objects = group.objects ?? []
  const style = kindStyle(group.kind)
  const { refresh, spec } = useTreeCtx()
  const label = kindLabel(spec, group.kind)
  const newLabel = `New ${label.replace(/s$/, '')}...`
  const menuItems = buildObjectGroupMenu({ newLabel, onRefresh: refresh })

  return (
    <div>
      <ContextMenu items={menuItems}>
        <TreeRow
          typeIcon={style.icon}
          typeIconClass={style.className}
          chevron={expanded}
          label={label}
          count={objects.length}
          onClick={() => setOpen(!expanded)}
        />
      </ContextMenu>
      {expanded && (
        <GuideChildren>
          {objects.map((ref) => (
            <SchemaObjectNode
              key={`${ref.kind}:${ref.name}`}
              objectRef={ref}
              forceOpen={forceOpen}
            />
          ))}
        </GuideChildren>
      )}
    </div>
  )
}

function SchemaObjectNode({ objectRef, forceOpen }: { objectRef: ObjectRef; forceOpen: boolean }) {
  const ctx = useContext(SchemaTreeContext)
  const [open, setOpen] = useState<boolean | null>(null)
  const expanded = open ?? forceOpen
  const detailQuery = useQuery({
    ...orgConnectionObjectQueryOptions(
      ctx!.orgSlug,
      ctx!.workspaceId,
      ctx!.connectionId,
      ctx!.sessionId,
      objectRef,
    ),
    enabled: Boolean(ctx) && expanded,
  })
  useEvictGoneSession(ctx?.connectionId, [detailQuery.error])
  const detail = detailQuery.data ?? null
  const columns = detail?.relational?.columns ?? []
  const insertable = useObjectInsert(objectRef)
  const { dialect, spec, openDiagram } = useTreeCtx()
  const style = kindStyle(objectRef.kind)
  const isView = objectRef.kind === 'view' || objectRef.kind === 'materialized_view'
  const objectMenu = buildObjectMenu({
    isView,
    onOpen: () => ctx?.openObject(objectRef),
    onViewDiagram:
      diagramSupportedForKind(spec, objectRef.kind) && openDiagram
        ? () => openDiagram({ kind: 'object', ref: objectRef })
        : undefined,
    onCopyName: () => copyWithToast(objectRef.name),
    onCopyQualifiedName: () =>
      copyWithToast(
        dialect ? dialect.formatObject(objectRef.namespace, objectRef.name) : objectRef.name,
      ),
    onCopyColumnList: () =>
      copyWithToast(
        columnList(columns.map((c) => (dialect ? dialect.formatColumn(c.name) : c.name))),
      ),
  })

  return (
    <div>
      <ContextMenu items={objectMenu}>
        <TreeRow
          typeIcon={style.icon}
          typeIconClass={style.className}
          chevron={expanded}
          label={objectRef.name}
          insertable={insertable}
          onClick={() => setOpen(!expanded)}
          onDoubleClickRow={() => ctx?.openObject(objectRef)}
        />
      </ContextMenu>
      {expanded && (
        <GuideChildren>
          <SchemaObjectDetail
            detail={detail}
            loading={detailQuery.isLoading}
            objectName={objectRef.name}
          />
        </GuideChildren>
      )}
    </div>
  )
}

function SchemaObjectDetail({
  detail,
  loading,
  objectName,
}: {
  detail: ObjectDetail | null
  loading: boolean
  objectName: string
}) {
  const { dialect } = useTreeCtx()
  if (loading && !detail) {
    return (
      <DetailMessage>
        <SchemaSpinner size={11} />
        Loading...
      </DetailMessage>
    )
  }
  const rel = detail?.relational
  if (!rel) {
    return <SchemaDescriptors descriptors={detail?.descriptors ?? []} />
  }
  const pk = new Set(rel.primary_key ?? [])
  const fk = new Set((rel.foreign_keys ?? []).flatMap((f) => f.columns))
  const columns = rel.columns ?? []
  const indexes = rel.indexes ?? []
  return (
    <>
      <DetailGroup label="Columns" count={columns.length} defaultOpen>
        {columns.map((c) => (
          <ContextMenu
            key={c.name}
            items={buildColumnMenu({
              onCopyName: () => copyWithToast(dialect ? dialect.formatColumn(c.name) : c.name),
              onCopyQualifiedName: () =>
                copyWithToast(
                  dialect
                    ? qualifiedColumn(dialect, objectName, c.name)
                    : `${objectName}.${c.name}`,
                ),
              onCopyType: () => copyWithToast(c.data_type),
            })}
          >
            <ColumnRow
              name={c.name}
              dataType={c.data_type}
              nullable={c.nullable}
              keyKind={pk.has(c.name) ? 'PK' : fk.has(c.name) ? 'FK' : undefined}
            />
          </ContextMenu>
        ))}
      </DetailGroup>
      {indexes.length > 0 && (
        <DetailGroup label="Indexes" count={indexes.length}>
          {indexes.map((ix) => (
            <ContextMenu
              key={`ix:${ix.name}`}
              items={buildIndexMenu({ onCopyName: () => copyWithToast(ix.name) })}
            >
              <LeafRow
                icon="key-01"
                iconClass="text-chart-4"
                label={ix.name}
                meta={ix.unique ? 'unique' : 'index'}
              />
            </ContextMenu>
          ))}
        </DetailGroup>
      )}
    </>
  )
}

/** Collapsible sub-section under an expanded object (Columns, Indexes, …). */
function DetailGroup({
  label,
  count,
  defaultOpen = false,
  children,
}: {
  label: string
  count: number
  defaultOpen?: boolean
  children: React.ReactNode
}) {
  const [open, setOpen] = useState(defaultOpen)
  return (
    <div>
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className={cn(
          'mx-1 flex h-[22px] w-[calc(100%-0.5rem)] items-center gap-1 rounded-md pl-1 pr-2 text-left',
          'text-[10px] font-semibold uppercase tracking-wider text-muted-foreground/80',
          'transition-colors hover:bg-sidebar-accent hover:text-foreground',
        )}
      >
        <Icon
          name={open ? 'chevron-down' : 'chevron-right'}
          size={10}
          className="shrink-0 text-muted-foreground/60"
        />
        <span className="truncate">{label}</span>
        <span className="font-normal tabular-nums text-muted-foreground/60">{count}</span>
      </button>
      {open && <GuideChildren>{children}</GuideChildren>}
    </div>
  )
}

function ColumnRow({
  name,
  dataType,
  nullable,
  keyKind,
}: {
  name: string
  dataType: string
  nullable?: boolean
  keyKind?: 'PK' | 'FK'
}) {
  const insertable = useColumnInsert(name)
  const icon = columnTypeIcon(dataType)
  return (
    <div
      className={cn(
        'mx-1 flex h-[22px] items-center gap-1.5 rounded-md pl-1 pr-2 text-[11px] transition-colors hover:bg-sidebar-accent/60',
        insertable && 'cursor-grab active:cursor-grabbing',
      )}
      draggable={insertable?.draggable}
      onDragStart={insertable?.onDragStart}
      onDoubleClick={insertable?.onDoubleClick}
    >
      <Icon
        name={icon}
        size={12}
        className={cn('shrink-0', columnTypeIconColor[icon] ?? 'text-muted-foreground')}
      />
      <span className={cn('min-w-0 flex-1 truncate', !insertable && 'select-text')} title={name}>
        {name}
      </span>
      {keyKind && <KeyBadge kind={keyKind} />}
      <span
        className="min-w-0 max-w-[45%] shrink truncate text-right font-mono text-[10px] text-muted-foreground/80"
        title={`${dataType}${nullable ? ' · nullable' : ''}`}
      >
        {dataType}
        {nullable ? '?' : ''}
      </span>
    </div>
  )
}

function SchemaDescriptors({ descriptors }: { descriptors: ObjectDescriptor[] }) {
  if (descriptors.length === 0) {
    return null
  }
  return (
    <>
      {descriptors.map((descriptor) => (
        <div key={`${descriptor.kind}:${descriptor.title}`}>
          <DetailMessage>{descriptor.title}</DetailMessage>
          <GuideChildren>
            {(descriptor.fields ?? []).map((field) => (
              <LeafRow
                key={`${descriptor.title}:${field.name}`}
                icon="box"
                label={field.name}
                meta={field.value}
              />
            ))}
            {descriptor.source ? (
              <LeafRow
                icon="terminal"
                label={descriptor.source.language.toUpperCase()}
                meta={descriptor.source.body}
              />
            ) : null}
            {descriptor.rows ? (
              <DetailMessage>{`${descriptor.rows.rows.length} rows`}</DetailMessage>
            ) : null}
          </GuideChildren>
        </div>
      ))}
    </>
  )
}

function DetailMessage({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex h-[22px] items-center gap-1.5 pl-2 pr-3 text-[11px] text-muted-foreground">
      {children}
    </div>
  )
}

function LeafRow({
  icon,
  iconClass,
  label,
  meta,
}: {
  icon: AppIcon
  iconClass?: string
  label: string
  meta: string
}) {
  return (
    <div className="mx-1 flex h-[22px] items-center gap-1.5 rounded-md pl-1 pr-2 text-[11px] transition-colors hover:bg-sidebar-accent/60">
      <Icon
        name={icon}
        size={12}
        className={cn('shrink-0', iconClass ?? 'text-muted-foreground')}
      />
      <span className="min-w-0 flex-1 select-text truncate" title={label}>
        {label}
      </span>
      <span
        className="min-w-0 max-w-[55%] shrink truncate pl-3 text-right font-mono text-[10px] text-muted-foreground/80"
        title={meta}
      >
        {meta}
      </span>
    </div>
  )
}

function TreeRow({
  typeIcon,
  typeIconClass,
  chevron,
  label,
  count,
  bold,
  insertable,
  onClick,
  onDoubleClickRow,
}: {
  typeIcon: AppIcon
  typeIconClass?: string
  chevron: boolean
  label: string
  count?: number
  bold?: boolean
  insertable?: InsertableProps
  onClick: () => void
  /** When set, double-clicking the row runs this instead of the insertable's
   *  insert action (object rows open their detail tab; drag-to-insert stays). */
  onDoubleClickRow?: () => void
}) {
  function handleClick(e: React.MouseEvent) {
    if (window.getSelection()?.toString()) return
    if (insertable && e.detail > 1) return
    onClick()
  }

  return (
    <div
      role="button"
      tabIndex={0}
      draggable={insertable?.draggable}
      onDragStart={insertable?.onDragStart}
      onDoubleClick={onDoubleClickRow ?? insertable?.onDoubleClick}
      onClick={handleClick}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault()
          onClick()
        }
      }}
      className="mx-1 flex h-6 cursor-pointer items-center gap-1.5 rounded-md pl-1 pr-2 text-left text-xs transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground"
    >
      <Icon
        name={chevron ? 'chevron-down' : 'chevron-right'}
        size={11}
        className="shrink-0 text-muted-foreground/70"
      />
      <Icon
        name={typeIcon}
        size={13}
        className={cn('shrink-0', typeIconClass ?? 'text-muted-foreground')}
      />
      <span
        className={cn(
          'min-w-0 flex-1 truncate',
          !insertable && 'select-text',
          bold && 'font-medium',
        )}
        title={label}
      >
        {label}
      </span>
      {count !== undefined && (
        <span className="shrink-0 tabular-nums text-[10px] text-muted-foreground/60">{count}</span>
      )}
    </div>
  )
}

function KeyBadge({ kind }: { kind: 'PK' | 'FK' }) {
  return (
    <span
      className={cn(
        'shrink-0 rounded px-1 text-[9px] font-semibold tracking-wide',
        kind === 'PK'
          ? 'bg-amber-500/15 text-amber-600 dark:text-amber-400'
          : 'bg-sky-500/15 text-sky-600 dark:text-sky-400',
      )}
    >
      {kind}
    </span>
  )
}
