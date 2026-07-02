import { Handle, Position, type NodeProps } from '@xyflow/react'
import { Icon } from '#/lib/icons'
import { cn } from '#/lib/utils'
import type { DbColumn, ObjectRef } from '#/lib/api/types'

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
  loading: boolean
  hasHidden: boolean
  onToggleCollapse: () => void
  onExpand: (refs: ObjectRef[]) => void
  onOpenDetail: () => void
}

const DOT = '!h-2 !w-2 !min-w-0 !rounded-full !border-border !bg-muted-foreground'

// Handles are anchored to the specific FK column rows (not the header): the
// left dot on a referenced column, the right dot on a foreign-key column. When
// the related table is off-canvas a small `+` chip next to that dot expands it.
// Collapsed/loading nodes fall back to the always-present node-level handles.
export function TableNode({ data }: NodeProps & { data: TableNodeData }) {
  const { outgoingByCol, incomingByCol } = data
  return (
    <div className="relative rounded border border-border bg-card text-xs shadow-sm" style={{ width: 240 }}>
      <Handle id="node:in" type="target" position={Position.Left} className={cn(DOT, '!opacity-0')} isConnectable={false} />
      <Handle id="node:out" type="source" position={Position.Right} className={cn(DOT, '!opacity-0')} isConnectable={false} />

      <div
        className="flex items-center gap-1 rounded-t border-b border-border bg-muted/60 px-2 py-1"
        onDoubleClick={data.onOpenDetail}
      >
        <button
          type="button"
          onClick={(e) => { e.stopPropagation(); data.onToggleCollapse() }}
          aria-label={data.collapsed ? 'Expand columns' : 'Collapse columns'}
          className="text-muted-foreground hover:text-foreground"
        >
          <Icon name={data.collapsed ? 'arrow-right-01' : 'arrow-down-01'} size={12} />
        </button>
        <span className="truncate font-medium text-foreground">{data.label}</span>
        {data.collapsed && data.hasHidden && (
          <span className="ml-auto h-1.5 w-1.5 shrink-0 rounded-full bg-primary/60" title="Has hidden relations" />
        )}
      </div>

      {!data.collapsed && (
        <div>
          {data.loading && <div className="px-2 py-1 text-[10px] text-muted-foreground">Loading columns…</div>}
          {!data.loading && data.columns.length === 0 && (
            <div className="px-2 py-1 text-[10px] text-muted-foreground">No columns</div>
          )}
          {data.columns.map((c) => {
            const out = outgoingByCol[c.name]
            const inc = incomingByCol[c.name]
            return (
              <div key={c.name} className="relative flex items-center gap-1 border-b border-border/50 px-2 py-0.5 last:border-b-0">
                {inc && <Handle id={`col:${c.name}:in`} type="target" position={Position.Left} className={cn(DOT, '!left-0')} isConnectable={false} />}
                {inc?.hidden && (
                  <button
                    type="button"
                    onClick={(e) => { e.stopPropagation(); data.onExpand(inc.sources) }}
                    className="absolute -left-2 z-10 flex h-3 w-3 items-center justify-center rounded-full border border-border bg-card text-[10px] leading-none text-primary hover:bg-primary/10"
                    title="Show referencing tables"
                  >
                    +
                  </button>
                )}
                <span className={cn('truncate', data.pk.has(c.name) ? 'font-semibold text-foreground' : 'text-muted-foreground')}>{c.name}</span>
                <span className="ml-auto shrink-0 font-mono text-[10px] text-muted-foreground">{c.data_type}</span>
                {data.pk.has(c.name) && <span className="shrink-0 text-[9px] text-amber-500">PK</span>}
                {out && <span className="shrink-0 text-[9px] text-blue-500">FK</span>}
                {out && <Handle id={`col:${c.name}:out`} type="source" position={Position.Right} className={cn(DOT, '!right-0')} isConnectable={false} />}
                {out?.hidden && (
                  <button
                    type="button"
                    onClick={(e) => { e.stopPropagation(); data.onExpand([out.target]) }}
                    className="absolute -right-2 z-10 flex h-3 w-3 items-center justify-center rounded-full border border-border bg-card text-[10px] leading-none text-primary hover:bg-primary/10"
                    title={`Show ${out.target.name}`}
                  >
                    +
                  </button>
                )}
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
