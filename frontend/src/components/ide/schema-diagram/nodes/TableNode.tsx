import { useEffect } from 'react'
import { Handle, NodeResizeControl, Position, ResizeControlVariant, useUpdateNodeInternals, type NodeProps } from '@xyflow/react'
import { Icon } from '#/lib/icons'
import { cn } from '#/lib/utils'
import { Button } from '#/components/ui/button'
import type { DbColumn, ObjectRef } from '#/lib/api/types'
import { refKey } from '../diagramModel'
import { Tip } from '../Tip'

export type HoverRelation = { source: string; target: string; column: string }

export type ColumnOutgoing = { target: ObjectRef; hidden: boolean }
export type ColumnIncoming = { hidden: boolean; sources: ObjectRef[] }

export type TableNodeData = {
  label: string
  namespace: string
  columns: DbColumn[]
  pk: Set<string>
  outgoingByCol: Record<string, ColumnOutgoing> // this column is an FK → target table
  incomingByCol: Record<string, ColumnIncoming> // this column is referenced by other tables
  collapsed: boolean
  keysOnly: boolean // render only PK/FK columns (plus a "N more" indicator)
  loading: boolean
  hasHidden: boolean
  onToggleCollapse: () => void
  onExpand: (refs: ObjectRef[], opts: { fromKey: string; direction: 'in' | 'out' }) => void
  onOpenDetail: () => void
  onRemove: () => void
  onHoverRelation: (rel: HoverRelation | null) => void
}

const DOT = '!h-2 !w-2 !min-w-0 !rounded-full !border-border !bg-muted-foreground'

// Handles are anchored to the specific FK column rows (not the header): the
// left dot on a referenced column, the right dot on a foreign-key column. When
// the related table is off-canvas a small `+` chip next to that dot expands it.
// Collapsed/loading nodes fall back to the always-present node-level handles.
// A column is a "key" if it's part of the primary key, is a foreign key, or is
// referenced by another table's foreign key — the columns keys-only mode keeps.
function isKeyColumn(name: string, data: TableNodeData): boolean {
  return data.pk.has(name) || Boolean(data.outgoingByCol[name]) || Boolean(data.incomingByCol[name])
}

export function TableNode({ id, data }: NodeProps & { data: TableNodeData }) {
  const { outgoingByCol, incomingByCol } = data
  const visibleColumns = data.keysOnly ? data.columns.filter((c) => isKeyColumn(c.name, data)) : data.columns
  const hiddenCount = data.columns.length - visibleColumns.length
  const updateNodeInternals = useUpdateNodeInternals()
  const handlesSignature = data.collapsed
    ? 'collapsed'
    : visibleColumns.map((column) => `${column.name}:${incomingByCol[column.name] ? 'in' : ''}:${outgoingByCol[column.name] ? 'out' : ''}`).join('|')

  useEffect(() => {
    updateNodeInternals(id)
  }, [handlesSignature, id, updateNodeInternals])

  return (
    <div className="group relative cursor-move rounded border border-border bg-card text-xs shadow-sm" style={{ width: '100%' }}>
      {/* Horizontal-only resizers on the left/right edges: transparent until you
          hover the edge itself, brighter while grabbing. No corner handles. */}
      <NodeResizeControl
        position="left"
        variant={ResizeControlVariant.Line}
        minWidth={160}
        className="!border-l-2 !border-primary/0 transition-colors hover:!border-primary/70"
        style={{ cursor: 'ew-resize' }}
      />
      <NodeResizeControl
        position="right"
        variant={ResizeControlVariant.Line}
        minWidth={160}
        className="!border-r-2 !border-primary/0 transition-colors hover:!border-primary/70"
        style={{ cursor: 'ew-resize' }}
      />
      <Handle id="node:in" type="target" position={Position.Left} className={cn(DOT, '!opacity-0')} isConnectable={false} />
      <Handle id="node:out" type="source" position={Position.Right} className={cn(DOT, '!opacity-0')} isConnectable={false} />

      <div
        className="flex items-center gap-1 rounded-t border-b border-border bg-muted/60 px-2 py-1"
        onDoubleClick={data.onOpenDetail}
      >
        <Button
          variant="ghost"
          size="icon-xs"
          onClick={(e) => { e.stopPropagation(); data.onToggleCollapse() }}
          aria-label={data.collapsed ? 'Expand columns' : 'Collapse columns'}
          className="text-muted-foreground"
        >
          <Icon name={data.collapsed ? 'arrow-right-01' : 'arrow-down-01'} size={12} />
        </Button>
        <span className="min-w-0 flex-1 truncate font-medium text-foreground">{data.label}</span>
        {data.collapsed && data.hasHidden && (
          <Tip label="Has hidden relations">
            <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-primary/60" />
          </Tip>
        )}
        <Tip label="Remove from diagram">
          <Button
            variant="ghost"
            size="icon-xs"
            onClick={(e) => { e.stopPropagation(); data.onRemove() }}
            aria-label="Remove from diagram"
            className="shrink-0 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100"
          >
            <Icon name="cancel-01" size={12} />
          </Button>
        </Tip>
      </div>

      {!data.collapsed && (
        <div>
          {data.loading && <div className="px-2 py-1 text-[10px] text-muted-foreground">Loading columns…</div>}
          {!data.loading && data.columns.length === 0 && (
            <div className="px-2 py-1 text-[10px] text-muted-foreground">No columns</div>
          )}
          {visibleColumns.map((c) => {
            const out = outgoingByCol[c.name]
            const inc = incomingByCol[c.name]
            // Hovering an FK column whose relation is on-canvas highlights the
            // referenced table + its edge.
            const rel: HoverRelation | null = out && !out.hidden ? { source: id, target: refKey(out.target), column: c.name } : null
            return (
              <div
                key={c.name}
                className={cn('relative flex items-center gap-1 border-b border-border/50 px-2 py-0.5 last:border-b-0', rel && 'hover:bg-primary/5')}
                onMouseEnter={rel ? () => data.onHoverRelation(rel) : undefined}
                onMouseLeave={rel ? () => data.onHoverRelation(null) : undefined}
              >
                {inc && <Handle id={`col:${c.name}:in`} type="target" position={Position.Left} className={cn(DOT, '!left-0')} isConnectable={false} />}
                {inc?.hidden && (
                  <Tip label="Add tables that reference this column" side="left">
                    <button
                      type="button"
                      onClick={(e) => { e.stopPropagation(); data.onExpand(inc.sources, { fromKey: id, direction: 'in' }) }}
                      className="absolute -left-2 top-1/2 z-10 flex h-3.5 w-3.5 -translate-y-1/2 cursor-pointer items-center justify-center rounded-full border border-border bg-card text-primary hover:bg-primary/10"
                    >
                      <Icon name="plus-sign" size={9} />
                    </button>
                  </Tip>
                )}
                <span className={cn('min-w-0 flex-1 truncate', data.pk.has(c.name) ? 'font-semibold text-foreground' : 'text-muted-foreground')}>{c.name}</span>
                {/* Native title (not the shared Tip) on purpose: this is one per
                    column across every node, so a Tooltip instance each would be
                    thousands on a large diagram — native title stays cheap. */}
                <span title={c.data_type} className="ml-1 max-w-[50%] shrink truncate font-mono text-[10px] text-muted-foreground">{c.data_type}</span>
                {data.pk.has(c.name) && <span className="shrink-0 text-[9px] font-medium text-chart-4">PK</span>}
                {out && <span className="shrink-0 text-[9px] font-medium text-chart-1">FK</span>}
                {out && <Handle id={`col:${c.name}:out`} type="source" position={Position.Right} className={cn(DOT, '!right-0')} isConnectable={false} />}
                {out?.hidden && (
                  <Tip label={`Add referenced table "${out.target.name}"`} side="right">
                    <button
                      type="button"
                      onClick={(e) => { e.stopPropagation(); data.onExpand([out.target], { fromKey: id, direction: 'out' }) }}
                      className="absolute -right-2 top-1/2 z-10 flex h-3.5 w-3.5 -translate-y-1/2 cursor-pointer items-center justify-center rounded-full border border-border bg-card text-primary hover:bg-primary/10"
                    >
                      <Icon name="plus-sign" size={9} />
                    </button>
                  </Tip>
                )}
              </div>
            )
          })}
          {hiddenCount > 0 && (
            <div className="px-2 py-0.5 text-[10px] italic text-muted-foreground">
              {hiddenCount} more {hiddenCount === 1 ? 'column' : 'columns'}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
