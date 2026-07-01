import { Handle, Position, type NodeProps } from '@xyflow/react'
import { Icon } from '#/lib/icons'
import { cn } from '#/lib/utils'
import type { DbColumn } from '#/lib/api/types'

export type TableNodeData = {
  label: string
  namespace: string
  columns: DbColumn[]
  pk: Set<string>
  fk: Set<string>
  hiddenCount: number
  collapsed: boolean
  loading: boolean
  onToggleCollapse: () => void
  onExpand: () => void
  onOpenDetail: () => void
}

// Node-level handles (not per-column) so foreign-key edges attach reliably even
// when a node is collapsed to title-only. Left = incoming, right = outgoing,
// matching elk's left-to-right layered layout.
export function TableNode({ data }: NodeProps & { data: TableNodeData }) {
  return (
    <div className="rounded border border-border bg-card text-xs shadow-sm" style={{ width: 240 }}>
      <Handle type="target" position={Position.Left} className="!h-2 !w-2 !border-border !bg-muted-foreground" />
      <Handle type="source" position={Position.Right} className="!h-2 !w-2 !border-border !bg-muted-foreground" />
      <div
        className="flex items-center gap-1 rounded-t border-b border-border bg-muted/60 px-2 py-1"
        onDoubleClick={data.onOpenDetail}
      >
        <button
          type="button"
          onClick={data.onToggleCollapse}
          aria-label={data.collapsed ? 'Expand columns' : 'Collapse columns'}
          className="text-muted-foreground hover:text-foreground"
        >
          <Icon name={data.collapsed ? 'arrow-right-01' : 'arrow-down-01'} size={12} />
        </button>
        <span className="truncate font-medium text-foreground">{data.label}</span>
        {data.hiddenCount > 0 && (
          <button
            type="button"
            onClick={data.onExpand}
            className="ml-auto shrink-0 rounded bg-primary/10 px-1 text-[10px] text-primary hover:bg-primary/20"
            aria-label="Expand related tables"
            title={`${data.hiddenCount} related table(s)`}
          >
            +{data.hiddenCount}
          </button>
        )}
      </div>
      {!data.collapsed && (
        <div>
          {data.loading && <div className="px-2 py-1 text-[10px] text-muted-foreground">Loading columns…</div>}
          {!data.loading && data.columns.length === 0 && (
            <div className="px-2 py-1 text-[10px] text-muted-foreground">No columns</div>
          )}
          {data.columns.map((c) => (
            <div key={c.name} className="flex items-center gap-1 border-b border-border/50 px-2 py-0.5 last:border-b-0">
              <span className={cn('truncate', data.pk.has(c.name) ? 'font-semibold text-foreground' : 'text-muted-foreground')}>{c.name}</span>
              <span className="ml-auto shrink-0 font-mono text-[10px] text-muted-foreground">{c.data_type}</span>
              {data.pk.has(c.name) && <span className="shrink-0 text-[9px] text-amber-500">PK</span>}
              {data.fk.has(c.name) && <span className="shrink-0 text-[9px] text-blue-500">FK</span>}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
