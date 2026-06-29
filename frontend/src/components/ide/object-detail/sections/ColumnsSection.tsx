import { columnTypeIcon } from '../../columnTypeIcon'
import { Icon } from '#/lib/icons'
import type { ColumnExtra, ObjectViewModel } from '../registry'

export function ColumnsSection({ vm, extras }: { vm: ObjectViewModel; extras: ColumnExtra[] }) {
  const rel = vm.detail.relational
  const columns = rel?.columns ?? []
  const pk = new Set(rel?.primary_key ?? [])
  const fk = new Set((rel?.foreign_keys ?? []).flatMap((f) => f.columns ?? []))

  if (columns.length === 0) {
    return <Empty>No columns.</Empty>
  }

  return (
    <table className="w-full border-separate border-spacing-0 text-xs">
      <thead className="sticky top-0 bg-muted/80 backdrop-blur-sm">
        <tr>
          <Th>Name</Th>
          <Th>Type</Th>
          <Th>Nullable</Th>
          <Th>Default</Th>
          <Th>Key</Th>
          {extras.map((e) => (
            <Th key={e.id}>{e.header}</Th>
          ))}
        </tr>
      </thead>
      <tbody>
        {columns.map((c) => (
          <tr key={c.name} className="hover:bg-accent/30">
            <Td>
              <span className="flex items-center gap-1.5">
                <Icon name={columnTypeIcon(c.data_type)} size={12} className="shrink-0 text-muted-foreground" />
                <span className="font-medium text-foreground">{c.name}</span>
              </span>
            </Td>
            <Td className="font-mono text-muted-foreground">{c.data_type}</Td>
            <Td className="text-muted-foreground">{c.nullable ? 'null' : 'not null'}</Td>
            <Td className="font-mono text-muted-foreground">{c.default ?? ''}</Td>
            <Td>{pk.has(c.name) ? <Badge>PK</Badge> : fk.has(c.name) ? <Badge>FK</Badge> : ''}</Td>
            {extras.map((e) => (
              <Td key={e.id} className="text-muted-foreground">{e.cell(c)}</Td>
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  )
}

function Th({ children }: { children: React.ReactNode }) {
  return <th className="border-b border-border px-3 py-1.5 text-left font-medium text-muted-foreground">{children}</th>
}

function Td({ children, className = '' }: { children: React.ReactNode; className?: string }) {
  return <td className={`border-b border-border px-3 py-1 ${className}`}>{children}</td>
}

function Badge({ children }: { children: React.ReactNode }) {
  return <span className="rounded bg-muted px-1 text-[9px] font-medium text-muted-foreground">{children}</span>
}

function Empty({ children }: { children: React.ReactNode }) {
  return <div className="p-4 text-xs text-muted-foreground">{children}</div>
}
