import type { DragEvent, ReactNode } from 'react'
import { ContextMenu } from '#/components/ui/context-menu'
import { Icon } from '#/lib/icons'
import { cn } from '#/lib/utils'
import type { ObjectRef, ScopePath } from '#/lib/api/types'
import { columnTypeIcon } from '../columnTypeIcon'
import { formatRelativeTime } from '../relativeTime'
import { OBJECT_REF_DND_MIME } from '../schema-diagram/dnd'
import { IDENTIFIER_DND_MIME } from '../sqlDialect'
import { navigatorIcon, type NavigatorIconStyle } from './icons'
import { refOfPath, type NavigatorRow } from './model'
import {
  columnOf,
  isObjectLevel,
  isObjectLevelFolder,
  navigatorMenu,
  refreshPathOf,
  type NavigatorActions,
} from './navigatorMenus'

export const NAVIGATOR_ROW_HEIGHT = 24
const INDENT = 12

export function formatRowCount(count: number): string {
  if (count < 1000) return `${count}`
  const [divisor, suffix] = count < 1_000_000 ? [1_000, 'K'] : [1_000_000, 'M']
  return `~${(count / divisor).toFixed(1)}${suffix}`
}

interface RowProps {
  row: NavigatorRow
  actions: NavigatorActions
  insert: (text: string) => void
  onToggle: (key: string, expanded: boolean) => void
  onRetry: (parent: ScopePath, folder: string) => void
  onConnect?: () => void
}

export function NavigatorRowView(props: RowProps) {
  const { row, actions } = props
  if (row.type === 'status') return <StatusRow {...props} row={row} />
  return (
    <ContextMenu items={navigatorMenu(row, actions)}>
      {row.type === 'folder' ? (
        <FolderRow {...props} row={row} />
      ) : row.type === 'object' ? (
        <ObjectRow {...props} row={row} />
      ) : (
        <LeafRow {...props} row={row} />
      )}
    </ContextMenu>
  )
}

function FolderRow({
  row,
  actions,
  onToggle,
}: RowProps & { row: Extract<NavigatorRow, { type: 'folder' }> }) {
  const child = actions.tree.nodes[row.folder.child]
  const objectLevel = isObjectLevelFolder(actions.tree, row.parent)
  return (
    <RowShell
      depth={row.depth}
      chevron={row.expanded}
      section={!objectLevel}
      icon={navigatorIcon(child?.icon ?? row.folder.child, objectLevel)}
      label={row.folder.label}
      title={row.listing ? `Loaded ${formatRelativeTime(row.listing.fetched_at)}` : undefined}
      badge={
        row.listing ? (
          <span className="shrink-0 font-normal tabular-nums text-[10px] text-muted-foreground/60">
            {row.listing.items.length}
          </span>
        ) : null
      }
      onToggle={() => onToggle(row.key, !row.expanded)}
      onRefresh={() => actions.refresh(refreshPathOf(row))}
    />
  )
}

function ObjectRow({
  row,
  actions,
  insert,
  onToggle,
}: RowProps & { row: Extract<NavigatorRow, { type: 'object' }> }) {
  const ref = refOfPath(row.item.path)
  const rowCount = row.item.attributes?.row_count
  const objectLevel = isObjectLevel(actions.tree, row.item.path)
  return (
    <RowShell
      depth={row.depth}
      chevron={row.expanded}
      compact={!objectLevel}
      icon={navigatorIcon(row.node.icon, objectLevel)}
      label={row.item.name}
      bold={row.item.current && row.node.scope}
      dimmed={row.item.system}
      meta={typeof rowCount === 'number' ? formatRowCount(rowCount) : undefined}
      drag={
        row.node.scope
          ? undefined
          : { text: actions.dialect.formatObject(ref.scope, ref.name), ref, insert }
      }
      onToggle={() => onToggle(row.key, !row.expanded)}
      onOpen={row.node.scope ? undefined : () => actions.openObject(ref)}
      onRefresh={() => actions.refresh(refreshPathOf(row))}
    />
  )
}

function LeafRow({
  row,
  actions,
  insert,
}: RowProps & { row: Extract<NavigatorRow, { type: 'leaf' }> }) {
  const refresh = () => actions.refresh(refreshPathOf(row))
  if (row.node?.column) {
    const column = columnOf(row.item)
    const typeIcon = columnTypeIcon(column.data_type)
    const attributes = row.item.attributes ?? {}
    const keyKind =
      attributes.primary_key === true ? 'PK' : attributes.foreign_key === true ? 'FK' : undefined
    return (
      <RowShell
        depth={row.depth}
        compact
        icon={{
          icon: typeIcon,
          className: 'text-muted-foreground',
        }}
        label={row.item.name}
        dimmed={row.item.system}
        badge={keyKind ? <KeyBadge kind={keyKind} /> : null}
        meta={`${column.data_type}${column.nullable ? '?' : ''}`}
        drag={{ text: actions.dialect.formatColumn(row.item.name), insert }}
        onRefresh={refresh}
      />
    )
  }
  const objectLevel = isObjectLevel(actions.tree, row.item.path)
  const ref = refOfPath(row.item.path)
  return (
    <RowShell
      depth={row.depth}
      compact={!objectLevel}
      icon={navigatorIcon(row.node?.icon ?? row.item.kind, objectLevel)}
      label={row.item.name}
      dimmed={row.item.system}
      drag={
        objectLevel
          ? { text: actions.dialect.formatObject(ref.scope, ref.name), ref, insert }
          : undefined
      }
      onOpen={objectLevel ? () => actions.openObject(ref) : undefined}
      onRefresh={refresh}
    />
  )
}

function StatusRow({
  row,
  onRetry,
  onConnect,
}: RowProps & { row: Extract<NavigatorRow, { type: 'status' }> }) {
  return (
    <div
      role="treeitem"
      aria-level={row.depth + 1}
      style={{ paddingLeft: row.depth * INDENT + 21 }}
      className="flex h-6 items-center gap-1.5 pr-2 text-[11px] text-muted-foreground"
    >
      {row.status === 'loading' && (
        <>
          <Icon name="loading-03" size={11} className="shrink-0 animate-spin" />
          Loading…
        </>
      )}
      {row.status === 'empty' && 'No objects.'}
      {row.status === 'error' && (
        <>
          Failed to load.
          <button
            type="button"
            className="underline hover:text-foreground"
            onClick={() => onRetry(row.parent, row.folder.kind)}
          >
            Retry
          </button>
        </>
      )}
      {row.status === 'session_required' &&
        (onConnect ? (
          <button
            type="button"
            className="font-medium text-primary hover:underline"
            onClick={onConnect}
          >
            Connect to load
          </button>
        ) : (
          'Connect to load'
        ))}
    </div>
  )
}

function startDrag(event: DragEvent, text: string, ref?: ObjectRef) {
  event.dataTransfer.setData(IDENTIFIER_DND_MIME, text)
  event.dataTransfer.setData('text/plain', text)
  if (ref) event.dataTransfer.setData(OBJECT_REF_DND_MIME, JSON.stringify(ref))
  event.dataTransfer.effectAllowed = 'copy'
  event.stopPropagation()
}

function RowShell({
  depth,
  chevron,
  section,
  compact,
  icon,
  label,
  bold,
  dimmed,
  meta,
  title,
  badge,
  drag,
  onToggle,
  onOpen,
  onRefresh,
}: {
  depth: number
  chevron?: boolean
  section?: boolean
  compact?: boolean
  icon: NavigatorIconStyle
  label: string
  bold?: boolean
  dimmed?: boolean
  meta?: string
  title?: string
  badge?: ReactNode
  drag?: { text: string; ref?: ObjectRef; insert: (text: string) => void }
  onToggle?: () => void
  onOpen?: () => void
  onRefresh: () => void
}) {
  return (
    <div
      role="treeitem"
      aria-level={depth + 1}
      aria-expanded={chevron}
      aria-label={label}
      tabIndex={0}
      title={title}
      draggable={drag !== undefined}
      onDragStart={drag ? (event) => startDrag(event, drag.text, drag.ref) : undefined}
      onClick={(event) => {
        if (window.getSelection()?.toString()) return
        if (drag && event.detail > 1) return
        onToggle?.()
      }}
      onDoubleClick={onOpen ?? (drag ? () => drag.insert(drag.text) : undefined)}
      onKeyDown={(event) => {
        if (event.target !== event.currentTarget) return
        if (event.key === 'Enter') {
          event.preventDefault()
          ;(onToggle ?? onOpen)?.()
        } else if (event.key === ' ' && onToggle) {
          event.preventDefault()
          onToggle()
        }
      }}
      style={{ paddingLeft: depth * INDENT + 4 }}
      className={cn(
        'group relative mx-1 flex h-6 cursor-pointer items-center gap-1.5 rounded-md pr-1 text-left text-xs transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground',
        dimmed && 'text-muted-foreground',
        compact && 'text-[11px]',
        section &&
          'text-[10px] font-semibold uppercase tracking-wider text-muted-foreground/80 hover:text-foreground',
        drag && 'active:cursor-grabbing',
      )}
    >
      {Array.from({ length: depth }, (_, level) => (
        <span
          key={level}
          aria-hidden="true"
          style={{ left: level * INDENT + 9 }}
          className="pointer-events-none absolute inset-y-0 w-px bg-sidebar-border"
        />
      ))}
      {chevron === undefined ? (
        <span className="size-[11px] shrink-0" aria-hidden="true" />
      ) : (
        <Icon
          name={chevron ? 'chevron-down' : 'chevron-right'}
          size={section ? 10 : 11}
          className="shrink-0 text-muted-foreground/70"
        />
      )}
      {section ? null : (
        <Icon
          name={icon.icon}
          size={compact ? 12 : 13}
          className={cn('shrink-0', icon.className)}
        />
      )}
      <span className={cn('min-w-0 flex-1 truncate', bold && 'font-semibold')}>{label}</span>
      {badge}
      {meta ? (
        <span className="min-w-0 max-w-[45%] shrink truncate text-right font-mono text-[10px] text-muted-foreground/80">
          {meta}
        </span>
      ) : null}
      <button
        type="button"
        aria-label={`Refresh ${label}`}
        className="hidden size-4 shrink-0 items-center justify-center rounded text-muted-foreground hover:text-foreground group-focus-within:flex group-hover:flex"
        onClick={(event) => {
          event.stopPropagation()
          onRefresh()
        }}
      >
        <Icon name="refresh" size={11} />
      </button>
    </div>
  )
}

function KeyBadge({ kind }: { kind: 'PK' | 'FK' }) {
  return (
    <span
      className={cn(
        'shrink-0 rounded px-1 text-[9px] font-semibold tracking-wide',
        kind === 'PK' ? 'bg-primary/15 text-primary' : 'bg-muted text-muted-foreground',
      )}
    >
      {kind}
    </span>
  )
}
