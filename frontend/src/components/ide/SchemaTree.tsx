import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Icon, type AppIcon } from '#/lib/icons'
import { cn } from '#/lib/utils'
import { isApiError } from '#/lib/api/errors'
import { orgConnectionSchemaQueryOptions } from '#/lib/api/query'
import type { DbNamespace, DbTable, DbView } from '#/lib/api/types'
import { useIde } from './useIdeStore'
import { filterSchema } from './schemaFilter'

const indent = (depth: number) => 6 + depth * 11

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
    return <SchemaMessage>{filtering ? 'No matches.' : 'No tables or views.'}</SchemaMessage>
  }

  return (
    <div className="min-w-max">
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
  const [open, setOpen] = useState(true)
  const expanded = forceOpen || open
  const tables = namespace.tables ?? []
  const views = namespace.views ?? []

  return (
    <div>
      <TreeRow depth={0} typeIcon="database" chevron={expanded} bold label={namespace.name} onClick={() => setOpen((v) => !v)} />
      {expanded && (
        <>
          {tables.map((t) => (
            <SchemaObjectNode key={`t:${t.name}`} object={t} kind="table" typeIcon="table" depth={1} forceOpen={forceOpen} />
          ))}
          {views.map((v) => (
            <SchemaObjectNode key={`v:${v.name}`} object={v} kind="view" typeIcon="eye" depth={1} forceOpen={forceOpen} />
          ))}
        </>
      )}
    </div>
  )
}

function SchemaObjectNode({
  object,
  kind,
  typeIcon,
  depth,
  forceOpen,
}: {
  object: DbTable | DbView
  kind: 'table' | 'view'
  typeIcon: AppIcon
  depth: number
  forceOpen: boolean
}) {
  const [open, setOpen] = useState(false)
  const expanded = forceOpen || open
  const columns = object.columns ?? []
  const table = kind === 'table' ? (object as DbTable) : null
  const pk = new Set(table?.primary_key ?? [])
  const fk = new Set((table?.foreign_keys ?? []).flatMap((f) => f.columns))
  const indexes = table?.indexes ?? []

  return (
    <div>
      <TreeRow depth={depth} typeIcon={typeIcon} chevron={expanded} label={object.name} onClick={() => setOpen((v) => !v)} />
      {expanded && (
        <>
          {columns.map((c) => (
            <LeafRow
              key={c.name}
              depth={depth + 1}
              icon="column"
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
      <span className="flex-1 select-text whitespace-nowrap">{label}</span>
      <span className="shrink-0 pl-3 text-muted-foreground">{meta}</span>
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
  onClick,
}: {
  depth: number
  typeIcon: AppIcon
  chevron: boolean
  label: string
  bold?: boolean
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
      <Icon name={chevron ? 'chevron-down' : 'chevron-right'} size={11} className="shrink-0 text-muted-foreground" />
      <Icon name={typeIcon} size={13} className="shrink-0 text-muted-foreground" />
      <span className={cn('flex-1 select-text whitespace-nowrap', bold && 'font-medium')}>{label}</span>
    </div>
  )
}

function KeyBadge({ children }: { children: React.ReactNode }) {
  return <span className="shrink-0 rounded bg-muted px-1 text-[9px] font-medium text-muted-foreground">{children}</span>
}
