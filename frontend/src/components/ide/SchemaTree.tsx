import { createContext, useContext, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Icon, type AppIcon } from '#/lib/icons'
import { cn } from '#/lib/utils'
import { isApiError } from '#/lib/api/errors'
import {
  orgConnectionCatalogQueryOptions,
  orgConnectionSchemaSpecQueryOptions,
  orgConnectionObjectQueryOptions,
} from '#/lib/api/query'
import type { CatalogNamespace, CatalogObjectGroup, Connection, ObjectDescriptor, ObjectDetail, ObjectRef, SchemaSpec, Workspace } from '#/lib/api/types'
import { useIde } from './useIdeStore'
import { newObjectTab } from './object-detail/objectTab'
import { filterCatalog, kindLabel, sortedGroups } from './schemaCatalog'
import { columnTypeIcon } from './columnTypeIcon'
import { dialectFor, IDENTIFIER_DND_MIME, type SqlDialect } from './sqlDialect'
import { useInsertIntoEditor } from './useInsertIntoEditor'
import { ContextMenu } from '#/components/ui/context-menu'
import { copyWithToast, columnList, qualifiedColumn } from './contextMenus/clipboard'
import { buildNamespaceMenu, buildObjectGroupMenu } from './contextMenus/schemaMenu'
import { buildObjectMenu } from './contextMenus/objectMenu'
import { buildColumnMenu, buildIndexMenu } from './contextMenus/columnMenu'

const indent = (depth: number) => 4 + depth * 10

type TreeCtx = {
  dialect: SqlDialect
  insert: (text: string) => void
  refresh: () => void
  openObject: (ref: ObjectRef) => void
  spec: SchemaSpec | undefined
  orgSlug: string
  workspaceId: number
  connectionId: number
  sessionId: string
}

const SchemaTreeContext = createContext<TreeCtx | null>(null)

type InsertableProps = {
  draggable: boolean
  onDragStart: (e: React.DragEvent) => void
  onDoubleClick: () => void
}

function dragPropsFor(ctx: { insert: (text: string) => void } | null, text: string | undefined): InsertableProps | undefined {
  if (!ctx || text === undefined) return undefined
  return {
    draggable: true,
    onDragStart: (e) => {
      e.dataTransfer.setData(IDENTIFIER_DND_MIME, text)
      e.dataTransfer.setData('text/plain', text)
      e.dataTransfer.effectAllowed = 'copy'
      e.stopPropagation()
    },
    onDoubleClick: () => ctx.insert(text),
  }
}

function useObjectInsert(namespace: string, name: string): InsertableProps | undefined {
  const ctx = useContext(SchemaTreeContext)
  return dragPropsFor(ctx, ctx ? ctx.dialect.formatObject(namespace, name) : undefined)
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
  }
}

const KIND_ICON: Record<string, AppIcon> = {
  table: 'table',
  view: 'eye',
  materialized_view: 'table',
  function: 'play',
  procedure: 'terminal',
  sequence: 'sort',
  trigger: 'flow-connection',
}

const kindIcon = (kind: string): AppIcon => KIND_ICON[kind] ?? 'box'

export function SchemaTree({
  orgSlug,
  workspaceId,
  connectionId,
  driver,
  filter,
}: {
  orgSlug: string
  workspaceId: number
  connectionId: number
  driver: string
  filter: string
}) {
  const sessionId = useIde((s) => s.sessions[connectionId])
  const openTab = useIde((s) => s.openTab)
  const insert = useInsertIntoEditor()
  const dialect = dialectFor(driver)

  const openObject = (ref: ObjectRef) =>
    openTab(newObjectTab({ id: connectionId, driver } as Connection, { id: workspaceId } as Workspace, ref))

  const catalogQuery = useQuery({
    ...orgConnectionCatalogQueryOptions(orgSlug, workspaceId, connectionId, sessionId ?? ''),
    enabled: Boolean(sessionId),
  })
  const specQuery = useQuery({
    ...orgConnectionSchemaSpecQueryOptions(orgSlug, workspaceId, connectionId, sessionId ?? ''),
    enabled: Boolean(sessionId),
  })

  if (!sessionId) return <SchemaMessage>Not connected.</SchemaMessage>
  if (catalogQuery.isLoading) {
    return (
      <SchemaMessage>
        <SchemaSpinner />
        Loading schema...
      </SchemaMessage>
    )
  }
  if (catalogQuery.isError) {
    if (isApiError(catalogQuery.error) && catalogQuery.error.status === 501) {
      return <SchemaMessage>This driver doesn&apos;t support schema inspection.</SchemaMessage>
    }
    return (
      <div className="flex items-center gap-2 py-1.5 pr-2 text-xs text-muted-foreground" style={{ paddingLeft: indent(0) }}>
        <span>Failed to load schema.</span>
        <button type="button" className="underline hover:text-foreground" onClick={() => catalogQuery.refetch()}>
          Retry
        </button>
      </div>
    )
  }

  const raw = catalogQuery.data?.catalog
  if (!raw) return <SchemaMessage>No schema.</SchemaMessage>

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
    refresh: () => {
      void catalogQuery.refetch()
    },
    openObject,
    spec,
    orgSlug,
    workspaceId,
    connectionId,
    sessionId,
  }

  return (
    <SchemaTreeContext.Provider value={ctx}>
      <div>
        {single
          ? sortedGroups(single, spec).map((g) => (
              <SchemaGroupNode key={g.kind} group={g} baseDepth={0} forceOpen={filtering} />
            ))
          : namespaces.map((ns) => <SchemaNamespaceNode key={ns.name} namespace={ns} forceOpen={filtering} />)}
      </div>
    </SchemaTreeContext.Provider>
  )
}

function SchemaMessage({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex items-center gap-1.5 py-1.5 pr-2 text-xs text-muted-foreground" style={{ paddingLeft: indent(0) }}>
      {children}
    </div>
  )
}

function SchemaSpinner({ size = 12 }: { size?: number }) {
  return <Icon name="loading-03" size={size} className="shrink-0 animate-spin text-muted-foreground" />
}

function SchemaNamespaceNode({ namespace, forceOpen }: { namespace: CatalogNamespace; forceOpen: boolean }) {
  const [open, setOpen] = useState<boolean | null>(null)
  const expanded = open ?? forceOpen
  const { refresh, spec } = useTreeCtx()
  const groups = sortedGroups(namespace, spec)
  const menuItems = buildNamespaceMenu({
    onCopyName: () => copyWithToast(namespace.name),
    onRefresh: refresh,
  })

  return (
    <div>
      <ContextMenu items={menuItems}>
        <TreeRow depth={0} typeIcon="database" chevron={expanded} bold label={namespace.name} onClick={() => setOpen(!expanded)} />
      </ContextMenu>
      {expanded && groups.map((g) => <SchemaGroupNode key={g.kind} group={g} baseDepth={1} forceOpen={forceOpen} />)}
    </div>
  )
}

function SchemaGroupNode({
  group,
  baseDepth,
  forceOpen,
}: {
  group: CatalogObjectGroup
  baseDepth: number
  forceOpen: boolean
}) {
  const [open, setOpen] = useState<boolean | null>(null)
  const expanded = open ?? forceOpen
  const objects = group.objects ?? []
  const icon = kindIcon(group.kind)
  const { refresh, spec } = useTreeCtx()
  const label = kindLabel(spec, group.kind)
  const newLabel = `New ${label.replace(/s$/, '')}...`
  const menuItems = buildObjectGroupMenu({ newLabel, onRefresh: refresh })

  return (
    <div>
      <ContextMenu items={menuItems}>
        <TreeRow
          depth={baseDepth}
          typeIcon={expanded ? 'folder-open' : 'folder'}
          chevron={expanded}
          label={`${label} (${objects.length})`}
          onClick={() => setOpen(!expanded)}
        />
      </ContextMenu>
      {expanded &&
        objects.map((ref) => (
          <SchemaObjectNode key={`${ref.kind}:${ref.name}`} objectRef={ref} typeIcon={icon} depth={baseDepth + 1} forceOpen={forceOpen} />
        ))}
    </div>
  )
}

function SchemaObjectNode({
  objectRef,
  typeIcon,
  depth,
  forceOpen,
}: {
  objectRef: ObjectRef
  typeIcon: AppIcon
  depth: number
  forceOpen: boolean
}) {
  const ctx = useContext(SchemaTreeContext)
  const [open, setOpen] = useState<boolean | null>(null)
  const expanded = open ?? forceOpen
  const detailQuery = useQuery({
    ...orgConnectionObjectQueryOptions(ctx!.orgSlug, ctx!.workspaceId, ctx!.connectionId, ctx!.sessionId, objectRef),
    enabled: Boolean(ctx) && expanded,
  })
  const detail = detailQuery.data ?? null
  const columns = detail?.relational?.columns ?? []
  const insertable = useObjectInsert(objectRef.namespace, objectRef.name)
  const { dialect } = useTreeCtx()
  const isView = objectRef.kind === 'view' || objectRef.kind === 'materialized_view'
  const objectMenu = buildObjectMenu({
    isView,
    onOpen: () => ctx?.openObject(objectRef),
    onCopyName: () => copyWithToast(objectRef.name),
    onCopyQualifiedName: () => copyWithToast(dialect ? dialect.formatObject(objectRef.namespace, objectRef.name) : objectRef.name),
    onCopyColumnList: () => copyWithToast(columnList(columns.map((c) => (dialect ? dialect.formatColumn(c.name) : c.name)))),
  })

  return (
    <div>
      <ContextMenu items={objectMenu}>
        <TreeRow
          depth={depth}
          typeIcon={typeIcon}
          chevron={expanded}
          label={objectRef.name}
          insertable={insertable}
          onClick={() => setOpen(!expanded)}
          onDoubleClickRow={() => ctx?.openObject(objectRef)}
        />
      </ContextMenu>
      {expanded && <SchemaObjectDetail detail={detail} loading={detailQuery.isLoading} depth={depth + 1} objectName={objectRef.name} />}
    </div>
  )
}

function SchemaObjectDetail({
  detail,
  loading,
  depth,
  objectName,
}: {
  detail: ObjectDetail | null
  loading: boolean
  depth: number
  objectName: string
}) {
  const { dialect } = useTreeCtx()
  if (loading && !detail) {
    return (
      <DetailMessage depth={depth}>
        <SchemaSpinner size={11} />
        Loading...
      </DetailMessage>
    )
  }
  const rel = detail?.relational
  if (!rel) {
    return <SchemaDescriptors descriptors={detail?.descriptors ?? []} depth={depth} />
  }
  const pk = new Set(rel.primary_key ?? [])
  const fk = new Set((rel.foreign_keys ?? []).flatMap((f) => f.columns))
  return (
    <>
      {(rel.columns ?? []).map((c) => (
        <ContextMenu
          key={c.name}
          items={buildColumnMenu({
            onCopyName: () => copyWithToast(dialect ? dialect.formatColumn(c.name) : c.name),
            onCopyQualifiedName: () => copyWithToast(dialect ? qualifiedColumn(dialect, objectName, c.name) : `${objectName}.${c.name}`),
            onCopyType: () => copyWithToast(c.data_type),
          })}
        >
          <LeafRow
            depth={depth}
            icon={columnTypeIcon(c.data_type)}
            label={c.name}
            meta={`${c.data_type}${c.nullable ? ' · null' : ''}`}
            badge={pk.has(c.name) ? 'PK' : fk.has(c.name) ? 'FK' : undefined}
            insertName={c.name}
          />
        </ContextMenu>
      ))}
      {(rel.indexes ?? []).map((ix) => (
        <ContextMenu key={`ix:${ix.name}`} items={buildIndexMenu({ onCopyName: () => copyWithToast(ix.name) })}>
          <LeafRow depth={depth} icon="key-01" label={ix.name} meta={ix.unique ? 'unique' : 'index'} />
        </ContextMenu>
      ))}
    </>
  )
}

function SchemaDescriptors({ descriptors, depth }: { descriptors: ObjectDescriptor[]; depth: number }) {
  if (descriptors.length === 0) {
    return null
  }
  return (
    <>
      {descriptors.map((descriptor) => (
        <div key={`${descriptor.kind}:${descriptor.title}`}>
          <DetailMessage depth={depth}>{descriptor.title}</DetailMessage>
          {(descriptor.fields ?? []).map((field) => (
            <LeafRow key={`${descriptor.title}:${field.name}`} depth={depth + 1} icon="box" label={field.name} meta={field.value} />
          ))}
          {descriptor.source ? (
            <LeafRow depth={depth + 1} icon="terminal" label={descriptor.source.language.toUpperCase()} meta={descriptor.source.body} />
          ) : null}
          {descriptor.rows ? (
            <DetailMessage depth={depth + 1}>{`${descriptor.rows.rows.length} rows`}</DetailMessage>
          ) : null}
        </div>
      ))}
    </>
  )
}

function DetailMessage({ depth, children }: { depth: number; children: React.ReactNode }) {
  return (
    <div className="flex h-5 items-center gap-1.5 pr-3 text-[11px] text-muted-foreground" style={{ paddingLeft: indent(depth) }}>
      {children}
    </div>
  )
}

function LeafRow({
  depth,
  icon,
  label,
  meta,
  badge,
  insertName,
}: {
  depth: number
  icon: AppIcon
  label: string
  meta: string
  badge?: string
  insertName?: string
}) {
  const insertable = useColumnInsert(insertName ?? '')
  const dnd = insertName ? insertable : undefined
  return (
    <div
      className={cn('flex h-5 w-full items-center gap-1.5 pr-3 text-[11px]', dnd && 'cursor-grab active:cursor-grabbing')}
      style={{ paddingLeft: indent(depth) }}
      draggable={dnd?.draggable}
      onDragStart={dnd?.onDragStart}
      onDoubleClick={dnd?.onDoubleClick}
    >
      <Icon name={icon} size={12} className="shrink-0 text-muted-foreground" />
      <span className={cn('min-w-0 flex-1 truncate', !dnd && 'select-text')} title={label}>
        {label}
      </span>
      <span className="min-w-0 max-w-[55%] shrink truncate pl-3 text-right text-muted-foreground" title={meta}>
        {meta}
      </span>
      {badge ? <KeyBadge>{badge}</KeyBadge> : null}
    </div>
  )
}

function TreeRow({
  depth,
  typeIcon,
  chevron,
  label,
  bold,
  leaf,
  insertable,
  onClick,
  onDoubleClickRow,
}: {
  depth: number
  typeIcon: AppIcon
  chevron: boolean
  label: string
  bold?: boolean
  leaf?: boolean
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
      style={{ paddingLeft: indent(depth) }}
      className="flex h-6 w-full cursor-pointer items-center gap-1.5 pr-3 text-left text-xs transition-colors hover:bg-accent hover:text-accent-foreground"
    >
      {leaf ? (
        <span className="w-[11px] shrink-0" />
      ) : (
        <Icon name={chevron ? 'chevron-down' : 'chevron-right'} size={11} className="shrink-0 text-muted-foreground" />
      )}
      <Icon name={typeIcon} size={13} className="shrink-0 text-muted-foreground" />
      <span className={cn('min-w-0 flex-1 truncate', !insertable && 'select-text', bold && 'font-medium')} title={label}>
        {label}
      </span>
    </div>
  )
}

function KeyBadge({ children }: { children: React.ReactNode }) {
  return <span className="shrink-0 rounded bg-muted px-1 text-[9px] font-medium text-muted-foreground">{children}</span>
}
