import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Icon, type AppIcon } from '#/lib/icons'
import { cn } from '#/lib/utils'
import { isApiError } from '#/lib/api/errors'
import { orgConnectionSchemaQueryOptions } from '#/lib/api/query'
import type { DbNamespace, DbObjectGroup, DbObject } from '#/lib/api/types'
import { useIde } from './useIdeStore'
import { filterSchema } from './schemaFilter'
import { columnTypeIcon } from './columnTypeIcon'

const indent = (depth: number) => 6 + depth * 11

// Icon per object-group kind. Unknown kinds (future drivers/Phase C) fall back
// to a generic icon so they still render natively without a code change here.
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
  filter,
}: {
  orgSlug: string
  workspaceId: number
  connectionId: number
  filter: string
}) {
  const sessionId = useIde((s) => s.sessions[connectionId])

  const schemaQuery = useQuery({
    ...orgConnectionSchemaQueryOptions(orgSlug, workspaceId, connectionId, sessionId ?? ''),
    enabled: Boolean(sessionId),
  })

  if (!sessionId) return <SchemaMessage>Not connected.</SchemaMessage>
  if (schemaQuery.isLoading) return <SchemaMessage>Loading schema…</SchemaMessage>
  if (schemaQuery.isError) {
    if (isApiError(schemaQuery.error) && schemaQuery.error.status === 501) {
      return <SchemaMessage>This driver doesn&apos;t support schema introspection.</SchemaMessage>
    }
    return (
      <div className="flex items-center gap-2 py-1.5 pr-2 text-xs text-muted-foreground" style={{ paddingLeft: indent(0) }}>
        <span>Failed to load schema.</span>
        <button type="button" className="underline hover:text-foreground" onClick={() => schemaQuery.refetch()}>
          Retry
        </button>
      </div>
    )
  }

  const raw = schemaQuery.data?.schema
  if (!raw) return <SchemaMessage>No schema.</SchemaMessage>

  const filtering = filter.trim() !== ''
  const namespaces = filterSchema(raw, filter).namespaces ?? []

  if (namespaces.length === 0) {
    return <SchemaMessage>{filtering ? 'No matches.' : 'No objects.'}</SchemaMessage>
  }

  return (
    <div>
      {namespaces.map((ns) => (
        <SchemaNamespaceNode key={ns.name} namespace={ns} forceOpen={filtering} />
      ))}
    </div>
  )
}

function SchemaMessage({ children }: { children: React.ReactNode }) {
  return (
    <div className="py-1.5 pr-2 text-xs text-muted-foreground" style={{ paddingLeft: indent(0) }}>
      {children}
    </div>
  )
}

function SchemaNamespaceNode({ namespace, forceOpen }: { namespace: DbNamespace; forceOpen: boolean }) {
  const [open, setOpen] = useState(false)
  const expanded = forceOpen || open
  const groups = (namespace.object_groups ?? []).filter((g) => (g.objects ?? []).length > 0)

  return (
    <div>
      <TreeRow depth={0} typeIcon="database" chevron={expanded} bold label={namespace.name} onClick={() => setOpen((v) => !v)} />
      {expanded && groups.map((g) => <SchemaGroupNode key={g.kind} group={g} forceOpen={forceOpen} />)}
    </div>
  )
}

function SchemaGroupNode({ group, forceOpen }: { group: DbObjectGroup; forceOpen: boolean }) {
  const [open, setOpen] = useState(false)
  const expanded = forceOpen || open
  const objects = group.objects ?? []
  const icon = kindIcon(group.kind)

  return (
    <div>
      <TreeRow
        depth={1}
        typeIcon={expanded ? 'folder-open' : 'folder'}
        chevron={expanded}
        label={`${group.label} (${objects.length})`}
        onClick={() => setOpen((v) => !v)}
      />
      {expanded &&
        objects.map((o) => (
          <SchemaObjectNode key={o.name} object={o} typeIcon={icon} depth={2} forceOpen={forceOpen} />
        ))}
    </div>
  )
}

function SchemaObjectNode({
  object,
  typeIcon,
  depth,
  forceOpen,
}: {
  object: DbObject
  typeIcon: AppIcon
  depth: number
  forceOpen: boolean
}) {
  const [open, setOpen] = useState(false)
  const columns = object.columns ?? []
  const indexes = object.indexes ?? []
  const hasChildren = columns.length > 0 || indexes.length > 0
  const expanded = hasChildren && (forceOpen || open)
  const pk = new Set(object.primary_key ?? [])
  const fk = new Set((object.foreign_keys ?? []).flatMap((f) => f.columns))

  return (
    <div>
      <TreeRow
        depth={depth}
        typeIcon={typeIcon}
        chevron={expanded}
        leaf={!hasChildren}
        label={object.name}
        onClick={() => hasChildren && setOpen((v) => !v)}
      />
      {expanded && (
        <>
          {columns.map((c) => (
            <LeafRow
              key={c.name}
              depth={depth + 1}
              icon={columnTypeIcon(c.data_type)}
              label={c.name}
              meta={`${c.data_type}${c.nullable ? ' · null' : ''}`}
              badge={pk.has(c.name) ? 'PK' : fk.has(c.name) ? 'FK' : undefined}
            />
          ))}
          {indexes.map((ix) => (
            <LeafRow key={`ix:${ix.name}`} depth={depth + 1} icon="key-01" label={ix.name} meta={ix.unique ? 'unique' : 'index'} />
          ))}
        </>
      )}
    </div>
  )
}

function LeafRow({
  depth,
  icon,
  label,
  meta,
  badge,
}: {
  depth: number
  icon: AppIcon
  label: string
  meta: string
  badge?: string
}) {
  return (
    <div className="flex h-5 w-full items-center gap-1.5 pr-3 text-[11px]" style={{ paddingLeft: indent(depth) }}>
      <Icon name={icon} size={12} className="shrink-0 text-muted-foreground" />
      {/* Name is primary — it keeps its space; the type yields and truncates first. */}
      <span className="min-w-0 flex-1 select-text truncate" title={label}>{label}</span>
      <span className="min-w-0 max-w-[55%] shrink truncate pl-3 text-right text-muted-foreground" title={meta}>{meta}</span>
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
  onClick,
}: {
  depth: number
  typeIcon: AppIcon
  chevron: boolean
  label: string
  bold?: boolean
  leaf?: boolean
  onClick: () => void
}) {
  // A plain click toggles; a drag that leaves text selected does not (so the
  // label can be highlighted and copied). Rendered as a div (not a button) so
  // the text is natively selectable.
  function handleClick() {
    if (window.getSelection()?.toString()) return
    onClick()
  }

  return (
    <div
      role="button"
      tabIndex={0}
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
      <span className={cn('min-w-0 flex-1 select-text truncate', bold && 'font-medium')} title={label}>{label}</span>
    </div>
  )
}

function KeyBadge({ children }: { children: React.ReactNode }) {
  return <span className="shrink-0 rounded bg-muted px-1 text-[9px] font-medium text-muted-foreground">{children}</span>
}
