import { createContext, useContext, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Icon, type AppIcon } from '#/lib/icons'
import { cn } from '#/lib/utils'
import { isApiError } from '#/lib/api/errors'
import {
  orgConnectionDirectoryQueryOptions,
  orgConnectionSchemaSpecQueryOptions,
  orgConnectionObjectQueryOptions,
  orgEffectivePermissionsQueryOptions,
} from '#/lib/api/query'
import { hasAnyPermission, permission } from '#/lib/permissions'
import type {
  Connection,
  ObjectGroup,
  ObjectDescriptor,
  ObjectDetail,
  ObjectRef,
  ScopePath,
  ScopeNode,
  SchemaEditRequest,
  SchemaEditSpec,
  SchemaSpec,
  StatementOperation,
  StatementSpec,
  Workspace,
} from '#/lib/api/types'
import { useIde } from './useIdeStore'
import { newObjectTab } from './object-detail/objectTab'
import { newDiagramTab, type DiagramTarget } from './schema-diagram/diagramTab'
import { diagramSupported, diagramSupportedForKind } from './schema-diagram/capability'
import { OBJECT_REF_DND_MIME } from './schema-diagram/dnd'
import {
  defaultCreateTableScope,
  filterDirectory,
  formatRowCount,
  hasDirectoryObjects,
  kindLabel,
  kindLabelSingular,
  sortedGroups,
} from './schemaDirectory'
import { scopeLabel } from '#/lib/api/scope'
import { columnTypeIcon, columnTypeIconColor } from './columnTypeIcon'
import { dialectFor, IDENTIFIER_DND_MIME, type SqlDialect } from './sqlDialect'
import { useInsertIntoEditor } from './useInsertIntoEditor'
import { ContextMenu } from '#/components/ui/context-menu'
import { copyWithToast, columnList, qualifiedColumn } from './contextMenus/clipboard'
import { buildNamespaceMenu, buildObjectGroupMenu } from './contextMenus/schemaMenu'
import { buildObjectMenu } from './contextMenus/objectMenu'
import { buildColumnMenu, buildIndexMenu } from './contextMenus/columnMenu'
import { useEvictGoneSession } from './sessionErrors'
import { useSchemaRefresh } from './useSchemaRefresh'
import { useSchemaEdit } from './useSchemaEdit'
import {
  canCreateTable,
  canDropColumn,
  canDropIndex,
  canDropObject,
  canDropScope,
  canRenameColumn,
  cascadeAvailable,
} from './schemaEditCapability'
import { statementOperationsFor } from './generateStatementCapability'
import { CreateTableDialog } from './schemaEdit/CreateTableDialog'
import { RenameColumnDialog } from './schemaEdit/RenameColumnDialog'
import { DropConfirmDialog } from './schemaEdit/DropConfirmDialog'
import { GenerateStatementDialog } from './GenerateStatementDialog'

type DropTarget =
  | { kind: 'scope'; scope: ScopePath; scopeKind: string }
  | { kind: 'object'; ref: ObjectRef }
  | { kind: 'column'; ref: ObjectRef; columnName: string }
  | { kind: 'index'; ref: ObjectRef; indexName: string }

type TreeCtx = {
  dialect: SqlDialect
  insert: (text: string) => void
  refresh: () => void
  openObject: (ref: ObjectRef) => void
  openDiagram: (target: DiagramTarget) => void
  spec: SchemaSpec | undefined
  statementSpec: StatementSpec | undefined
  orgSlug: string
  workspaceId: number
  connectionId: number
  sessionId?: string
  editor: SchemaEditSpec | undefined
  canMutate: boolean
  openCreateTable: (scope: ScopePath) => void
  openDropScope: (scope: ScopePath, scopeKind: string) => void
  openDropObject: (ref: ObjectRef) => void
  openRenameColumn: (ref: ObjectRef, columnName: string) => void
  openDropColumn: (ref: ObjectRef, columnName: string) => void
  openDropIndex: (ref: ObjectRef, indexName: string) => void
  openGenerateStatement: (ref: ObjectRef, operation: StatementOperation) => void
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
  return dragPropsFor(ctx, ctx ? ctx.dialect.formatObject(ref.scope, ref.name) : undefined, ref)
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
    statementSpec: ctx?.statementSpec,
    openDiagram: ctx?.openDiagram,
    editor: ctx?.editor,
    sessionId: ctx?.sessionId,
    canMutate: ctx?.canMutate ?? false,
    openCreateTable: ctx?.openCreateTable,
    openDropScope: ctx?.openDropScope,
    openDropObject: ctx?.openDropObject,
    openRenameColumn: ctx?.openRenameColumn,
    openDropColumn: ctx?.openDropColumn,
    openDropIndex: ctx?.openDropIndex,
    openGenerateStatement: ctx?.openGenerateStatement,
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

const NON_EXPANDABLE_OBJECT_KINDS = new Set(['function', 'procedure', 'sequence'])

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

  const directoryQuery = useQuery({
    ...orgConnectionDirectoryQueryOptions(orgSlug, workspaceId, connectionId, sessionId),
  })
  const specQuery = useQuery({
    ...orgConnectionSchemaSpecQueryOptions(orgSlug, workspaceId, connectionId, sessionId),
  })
  const permissionsQuery = useQuery({
    ...orgEffectivePermissionsQueryOptions(orgSlug, 'connection', connectionId),
  })
  const refreshSchema = useSchemaRefresh({
    orgSlug,
    workspaceId,
    connectionId,
    sessionId,
  })
  const schemaEdit = useSchemaEdit({ orgSlug, workspaceId, connectionId, sessionId })

  const [createTableScope, setCreateTableScope] = useState<ScopePath | null>(null)
  const [renameTarget, setRenameTarget] = useState<{ ref: ObjectRef; columnName: string } | null>(
    null,
  )
  const [dropTarget, setDropTarget] = useState<DropTarget | null>(null)
  const [generateTarget, setGenerateTarget] = useState<{
    ref: ObjectRef
    operation: StatementOperation
  } | null>(null)

  // A 410 from any schema endpoint means the server-side session died (idle
  // timeout, restart). Drop it so the tree flips to the reconnect hint instead
  // of erroring forever against a dead session id.
  useEvictGoneSession(connectionId, [directoryQuery.error, specQuery.error])

  if (directoryQuery.isLoading) {
    return (
      <SchemaMessage>
        <SchemaSpinner />
        Loading schema...
      </SchemaMessage>
    )
  }
  if (directoryQuery.isError) {
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
    if (isApiError(directoryQuery.error) && directoryQuery.error.status === 501) {
      return <SchemaMessage>This driver doesn&apos;t support schema inspection.</SchemaMessage>
    }
    return (
      <div className="flex items-center gap-2 px-2 py-1.5 text-xs text-muted-foreground">
        <span>Failed to load schema.</span>
        <button
          type="button"
          className="underline hover:text-foreground"
          onClick={() => directoryQuery.refetch()}
        >
          Retry
        </button>
      </div>
    )
  }

  const raw = directoryQuery.data?.directory
  if (!raw) {
    if (directoryQuery.data?.status === 'pending') {
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
  const roots = filterDirectory(raw, filter).roots
  const noScope = roots.length === 0
  // A single unambiguous empty scope (one root, at most one child) collapses to
  // the flat empty state below. A directory with multiple schemas/databases
  // still has distinct scopes worth showing and right-clicking even when none
  // of them have objects yet, so it falls through to the tree render instead.
  const trivialEmptyScope =
    !filtering && !noScope && !hasDirectoryObjects(roots) && defaultCreateTableScope(roots) !== null
  const isEmpty = noScope || trivialEmptyScope

  const spec = specQuery.data?.spec
  const statementSpec = specQuery.data?.statements
  const editor = specQuery.data?.editor
  const canMutate = hasAnyPermission(permissionsQuery.data?.permissions, [
    permission.connExecute,
    permission.connDdl,
  ])
  const single = roots.length === 1 ? roots[0] : null

  // An empty scope is still a valid place to start: offer the create-table
  // action when the backend editor spec advertises it for this scope, rather
  // than dead-ending the explorer.
  const emptyCreateScope = isEmpty && !filtering ? defaultCreateTableScope(roots) : null
  const emptyCreateScopeKind = emptyCreateScope?.[emptyCreateScope.length - 1]?.kind ?? ''
  const emptyCreateGate = emptyCreateScope
    ? canCreateTable(editor, sessionId, canMutate, emptyCreateScopeKind)
    : null

  const ctx: TreeCtx = {
    dialect,
    insert,
    refresh: () => refreshSchema.mutate(),
    openObject,
    openDiagram,
    spec,
    statementSpec,
    orgSlug,
    workspaceId,
    connectionId,
    sessionId,
    editor,
    canMutate,
    openCreateTable: setCreateTableScope,
    openDropScope: (scope, scopeKind) => setDropTarget({ kind: 'scope', scope, scopeKind }),
    openDropObject: (ref) => setDropTarget({ kind: 'object', ref }),
    openRenameColumn: (ref, columnName) => setRenameTarget({ ref, columnName }),
    openDropColumn: (ref, columnName) => setDropTarget({ kind: 'column', ref, columnName }),
    openDropIndex: (ref, indexName) => setDropTarget({ kind: 'index', ref, indexName }),
    openGenerateStatement: (ref, operation) => setGenerateTarget({ ref, operation }),
  }

  return (
    <SchemaTreeContext.Provider value={ctx}>
      {isEmpty ? (
        <SchemaEmptyState
          filtering={filtering}
          onCreateTable={
            emptyCreateGate?.allowed && emptyCreateScope
              ? () => setCreateTableScope(emptyCreateScope)
              : undefined
          }
        />
      ) : (
        <div className="py-0.5">
          {single && (single.children?.length ?? 0) === 0 && single.groups.length > 0
            ? sortedGroups(single, spec).map((g) => (
                <SchemaGroupNode key={g.kind} group={g} scope={single.path} forceOpen={filtering} />
              ))
            : single && single.groups.length === 0 && (single.children?.length ?? 0) > 0
              ? (single.children ?? []).map((node) => (
                  <SchemaScopeNode
                    key={JSON.stringify(node.path)}
                    node={node}
                    forceOpen={filtering}
                  />
                ))
              : roots.map((node) => (
                  <SchemaScopeNode
                    key={JSON.stringify(node.path)}
                    node={node}
                    forceOpen={filtering}
                  />
                ))}
        </div>
      )}

      {editor && (
        <CreateTableDialog
          open={createTableScope !== null}
          onOpenChange={(open) => !open && setCreateTableScope(null)}
          scope={createTableScope ?? []}
          columnTypes={editor.column_types}
          pending={schemaEdit.isPending}
          onSubmit={(name, columns) =>
            schemaEdit.mutate(
              {
                operation: 'create_table',
                scope: createTableScope ?? [],
                name,
                columns,
              },
              { onSuccess: () => setCreateTableScope(null) },
            )
          }
        />
      )}

      {renameTarget && (
        <RenameColumnDialog
          open={renameTarget !== null}
          onOpenChange={(open) => !open && setRenameTarget(null)}
          tableName={renameTarget.ref.name}
          columnName={renameTarget.columnName}
          pending={schemaEdit.isPending}
          onSubmit={(newName) =>
            schemaEdit.mutate(
              {
                operation: 'rename_column',
                ref: renameTarget.ref,
                name: renameTarget.columnName,
                new_name: newName,
              },
              { onSuccess: () => setRenameTarget(null) },
            )
          }
        />
      )}

      {dropTarget && (
        <DropConfirmDialog
          open={dropTarget !== null}
          onOpenChange={(open) => !open && setDropTarget(null)}
          objectType={dropTargetLabel(dropTarget, spec)}
          objectName={dropTargetName(dropTarget)}
          description={dropTargetDescription(dropTarget, spec)}
          cascadeAvailable={cascadeAvailable(editor, dropTarget.kind)}
          pending={schemaEdit.isPending}
          onConfirm={(cascade) =>
            schemaEdit.mutate(dropTargetRequest(dropTarget, cascade), {
              onSuccess: () => setDropTarget(null),
            })
          }
        />
      )}

      <GenerateStatementDialog
        open={generateTarget !== null}
        onOpenChange={(open) => !open && setGenerateTarget(null)}
        orgSlug={orgSlug}
        workspaceId={workspaceId}
        connectionId={connectionId}
        sessionId={sessionId}
        target={generateTarget}
      />
    </SchemaTreeContext.Provider>
  )
}

function dropTargetLabel(target: DropTarget, spec: SchemaSpec | undefined): string {
  switch (target.kind) {
    case 'scope':
      return kindLabelSingular(spec, target.scopeKind).toLowerCase()
    case 'object':
      return kindLabelSingular(spec, target.ref.kind).toLowerCase()
    case 'column':
      return 'column'
    case 'index':
      return 'index'
  }
}

function dropTargetName(target: DropTarget): string {
  switch (target.kind) {
    case 'scope':
      return scopeLabel(target.scope)
    case 'object':
      return target.ref.name
    case 'column':
      return target.columnName
    case 'index':
      return target.indexName
  }
}

function dropTargetDescription(target: DropTarget, spec: SchemaSpec | undefined): string {
  switch (target.kind) {
    case 'scope':
      return `This permanently drops ${scopeLabel(target.scope)} and everything it contains. This cannot be undone.`
    case 'object':
      return `This permanently drops the ${kindLabelSingular(spec, target.ref.kind).toLowerCase()} ${target.ref.name}. This cannot be undone.`
    case 'column':
      return `This permanently drops the column ${target.columnName} from ${target.ref.name}, including its data. This cannot be undone.`
    case 'index':
      return `This permanently drops the index ${target.indexName} from ${target.ref.name}. This cannot be undone.`
  }
}

function dropTargetRequest(target: DropTarget, cascade: boolean): SchemaEditRequest {
  switch (target.kind) {
    case 'scope':
      return { operation: 'drop_scope', scope: target.scope, cascade }
    case 'object':
      return { operation: 'drop_object', ref: target.ref, cascade }
    case 'column':
      return { operation: 'drop_column', ref: target.ref, name: target.columnName, cascade }
    case 'index':
      return { operation: 'drop_index', ref: target.ref, name: target.indexName, cascade }
  }
}

function SchemaMessage({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex items-center gap-1.5 px-2 py-1.5 text-xs text-muted-foreground">
      {children}
    </div>
  )
}

function SchemaEmptyState({
  filtering,
  onCreateTable,
}: {
  filtering: boolean
  onCreateTable?: () => void
}) {
  return (
    <SchemaMessage>
      <span>{filtering ? 'No matches.' : 'No objects.'}</span>
      {onCreateTable && (
        <button
          type="button"
          className="font-medium text-primary hover:underline"
          onClick={onCreateTable}
        >
          Create table
        </button>
      )}
    </SchemaMessage>
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

function SchemaScopeNode({ node, forceOpen }: { node: ScopeNode; forceOpen: boolean }) {
  const [open, setOpen] = useState<boolean | null>(null)
  const expanded = open ?? forceOpen
  const {
    refresh,
    spec,
    openDiagram,
    editor,
    sessionId,
    canMutate,
    openCreateTable,
    openDropScope,
  } = useTreeCtx()
  const groups = sortedGroups(node, spec)
  const scopeKind = node.path[node.path.length - 1]?.kind ?? ''
  const label = node.path[node.path.length - 1]?.name ?? scopeLabel(node.path)
  const createGate = canCreateTable(editor, sessionId, canMutate, scopeKind)
  const dropGate = canDropScope(editor, sessionId, canMutate, scopeKind)
  const menuItems = buildNamespaceMenu({
    onCopyName: () => copyWithToast(label),
    onRefresh: refresh,
    onViewDiagram:
      diagramSupported(spec) && openDiagram
        ? () => openDiagram({ kind: 'scope', scope: node.path })
        : undefined,
    dropLabel: `Drop ${kindLabelSingular(spec, scopeKind).toLowerCase()}`,
    onCreateTable: createGate.allowed ? () => openCreateTable?.(node.path) : undefined,
    createTableDisabledReason: createGate.allowed ? undefined : createGate.reason,
    onDropScope: dropGate.allowed ? () => openDropScope?.(node.path, scopeKind) : undefined,
    dropScopeDisabledReason: dropGate.allowed ? undefined : dropGate.reason,
  })

  return (
    <div>
      <ContextMenu items={menuItems}>
        <TreeRow
          typeIcon="database"
          chevron={expanded}
          bold
          label={label}
          onClick={() => setOpen(!expanded)}
        />
      </ContextMenu>
      {expanded && (
        <GuideChildren>
          {groups.map((g) => (
            <SchemaGroupNode key={g.kind} group={g} scope={node.path} forceOpen={forceOpen} />
          ))}
          {(node.children ?? []).map((child) => (
            <SchemaScopeNode key={JSON.stringify(child.path)} node={child} forceOpen={forceOpen} />
          ))}
        </GuideChildren>
      )}
    </div>
  )
}

function SchemaGroupNode({
  group,
  scope,
  forceOpen,
}: {
  group: ObjectGroup
  scope: ScopePath
  forceOpen: boolean
}) {
  const [open, setOpen] = useState<boolean | null>(null)
  const expanded = open ?? forceOpen
  const objects = group.objects ?? []
  const style = kindStyle(group.kind)
  const { refresh, spec, openDiagram, editor, sessionId, canMutate, openCreateTable } = useTreeCtx()
  const label = kindLabel(spec, group.kind)
  const newLabel = `New ${label.replace(/s$/, '')}...`
  const scopeKind = scope[scope.length - 1]?.kind ?? ''
  const createGate =
    group.kind === 'table' ? canCreateTable(editor, sessionId, canMutate, scopeKind) : null
  const menuItems = buildObjectGroupMenu({
    newLabel,
    onRefresh: refresh,
    onViewDiagram:
      group.kind === 'table' && diagramSupportedForKind(spec, group.kind) && openDiagram
        ? () => openDiagram({ kind: 'scope', scope })
        : undefined,
    onCreateTable: createGate?.allowed ? () => openCreateTable?.(scope) : undefined,
    createTableDisabledReason: createGate && !createGate.allowed ? createGate.reason : undefined,
  })

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
              rowCount={group.row_counts?.[ref.name]}
            />
          ))}
        </GuideChildren>
      )}
    </div>
  )
}

function SchemaObjectNode({
  objectRef,
  forceOpen,
  rowCount,
}: {
  objectRef: ObjectRef
  forceOpen: boolean
  rowCount?: number
}) {
  const ctx = useContext(SchemaTreeContext)
  const [open, setOpen] = useState<boolean | null>(null)
  const expandable = !NON_EXPANDABLE_OBJECT_KINDS.has(objectRef.kind)
  const inlineDetail = objectRef.kind === 'sequence'
  const expanded = expandable && (open ?? forceOpen)
  const detailQuery = useQuery({
    ...orgConnectionObjectQueryOptions(
      ctx!.orgSlug,
      ctx!.workspaceId,
      ctx!.connectionId,
      ctx!.sessionId,
      objectRef,
    ),
    enabled: Boolean(ctx) && (expanded || inlineDetail),
  })
  useEvictGoneSession(ctx?.connectionId, [detailQuery.error])
  const detail = detailQuery.data ?? null
  const columns = detail?.relational?.columns ?? []
  const insertable = useObjectInsert(objectRef)
  const {
    dialect,
    spec,
    statementSpec,
    openDiagram,
    editor,
    sessionId,
    canMutate,
    openDropObject,
    openGenerateStatement,
  } = useTreeCtx()
  const style = kindStyle(objectRef.kind)
  const inlineMeta = inlineDetail
    ? sequenceDataType(detail)
    : rowCount !== undefined
      ? formatRowCount(rowCount)
      : undefined
  const isView = objectRef.kind === 'view' || objectRef.kind === 'materialized_view'
  const dropGate = canDropObject(editor, sessionId, canMutate, objectRef.kind)
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
        dialect ? dialect.formatObject(objectRef.scope, objectRef.name) : objectRef.name,
      ),
    onCopyColumnList: () =>
      copyWithToast(
        columnList(columns.map((c) => (dialect ? dialect.formatColumn(c.name) : c.name))),
      ),
    onDrop: dropGate.allowed ? () => openDropObject?.(objectRef) : undefined,
    dropDisabledReason: dropGate.allowed ? undefined : dropGate.reason,
    statementOperations: statementOperationsFor(statementSpec, objectRef.kind),
    onGenerateStatement: openGenerateStatement
      ? (operation) => openGenerateStatement(objectRef, operation)
      : undefined,
  })

  return (
    <div>
      <ContextMenu items={objectMenu}>
        <TreeRow
          typeIcon={style.icon}
          typeIconClass={style.className}
          chevron={expandable ? expanded : undefined}
          label={objectRef.name}
          meta={inlineMeta}
          insertable={insertable}
          onClick={expandable ? () => setOpen(!expanded) : undefined}
          onDoubleClickRow={() => ctx?.openObject(objectRef)}
        />
      </ContextMenu>
      {expanded && (
        <GuideChildren>
          <SchemaObjectDetail
            detail={detail}
            loading={detailQuery.isLoading}
            objectRef={objectRef}
          />
        </GuideChildren>
      )}
    </div>
  )
}

function SchemaObjectDetail({
  detail,
  loading,
  objectRef,
}: {
  detail: ObjectDetail | null
  loading: boolean
  objectRef: ObjectRef
}) {
  const { dialect, editor, sessionId, canMutate, openRenameColumn, openDropColumn, openDropIndex } =
    useTreeCtx()
  const objectName = objectRef.name
  const objectKind = objectRef.kind
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
    if (objectKind === 'trigger') {
      return <SchemaTriggerDetail descriptors={detail?.descriptors ?? []} />
    }
    return <SchemaDescriptors descriptors={detail?.descriptors ?? []} />
  }
  const pk = new Set(rel.primary_key ?? [])
  const fk = new Set((rel.foreign_keys ?? []).flatMap((f) => f.columns))
  const columns = rel.columns ?? []
  const indexes = rel.indexes ?? []
  const renameGate = canRenameColumn(editor, sessionId, canMutate, objectKind)
  const dropColumnGate = canDropColumn(editor, sessionId, canMutate, objectKind)
  const dropIndexGate = canDropIndex(editor, sessionId, canMutate, objectKind)
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
              onRename: renameGate.allowed
                ? () => openRenameColumn?.(objectRef, c.name)
                : undefined,
              renameDisabledReason: renameGate.allowed ? undefined : renameGate.reason,
              onDrop: dropColumnGate.allowed
                ? () => openDropColumn?.(objectRef, c.name)
                : undefined,
              dropDisabledReason: dropColumnGate.allowed ? undefined : dropColumnGate.reason,
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
              items={buildIndexMenu({
                onCopyName: () => copyWithToast(ix.name),
                onDrop: dropIndexGate.allowed
                  ? () => openDropIndex?.(objectRef, ix.name)
                  : undefined,
                dropDisabledReason: dropIndexGate.allowed ? undefined : dropIndexGate.reason,
              })}
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

function sequenceDataType(detail: ObjectDetail | null): string | undefined {
  return detail?.descriptors
    ?.flatMap((descriptor) => descriptor.fields ?? [])
    .find((field) => field.name.toLowerCase() === 'data type')?.value
}

function SchemaTriggerDetail({ descriptors }: { descriptors: ObjectDescriptor[] }) {
  const fields = descriptors
    .flatMap((descriptor) => descriptor.fields ?? [])
    .filter((field) => ['timing', 'event', 'table'].includes(field.name.toLowerCase()))

  return fields.map((field) => (
    <LeafRow key={field.name} icon="information-circle" label={field.name} meta={field.value} />
  ))
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
  meta,
  count,
  bold,
  insertable,
  onClick,
  onDoubleClickRow,
}: {
  typeIcon: AppIcon
  typeIconClass?: string
  chevron?: boolean
  label: string
  meta?: string
  count?: number
  bold?: boolean
  insertable?: InsertableProps
  onClick?: () => void
  /** When set, double-clicking the row runs this instead of the insertable's
   *  insert action (object rows open their detail tab; drag-to-insert stays). */
  onDoubleClickRow?: () => void
}) {
  function handleClick(e: React.MouseEvent) {
    if (window.getSelection()?.toString()) return
    if (insertable && e.detail > 1) return
    onClick?.()
  }

  return (
    <div
      role="button"
      aria-expanded={chevron === undefined ? undefined : chevron}
      tabIndex={0}
      draggable={insertable?.draggable}
      onDragStart={insertable?.onDragStart}
      onDoubleClick={onDoubleClickRow ?? insertable?.onDoubleClick}
      onClick={handleClick}
      onKeyDown={(e) => {
        if (e.key === 'Enter') {
          e.preventDefault()
          if (onClick) onClick()
          else onDoubleClickRow?.()
        } else if (e.key === ' ' && onClick) {
          e.preventDefault()
          onClick()
        }
      }}
      className="mx-1 flex h-6 cursor-pointer items-center gap-1.5 rounded-md pl-1 pr-2 text-left text-xs transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground"
    >
      {chevron === undefined ? (
        <span className="size-[11px] shrink-0" aria-hidden="true" />
      ) : (
        <Icon
          name={chevron ? 'chevron-down' : 'chevron-right'}
          size={11}
          className="shrink-0 text-muted-foreground/70"
        />
      )}
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
      {meta ? (
        <span
          className="min-w-0 max-w-[45%] shrink truncate text-right font-mono text-[10px] text-muted-foreground/80"
          title={meta}
        >
          {meta}
        </span>
      ) : null}
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
