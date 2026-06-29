import type { ObjectViewModel } from '../registry'

export function KeysSection({ vm }: { vm: ObjectViewModel }) {
  const rel = vm.detail.relational
  if (!rel) {
    return <DescriptorList vm={vm} />
  }
  const pk = rel.primary_key ?? []
  const fks = rel.foreign_keys ?? []
  const indexes = rel.indexes ?? []

  if (pk.length === 0 && fks.length === 0 && indexes.length === 0) {
    return <Empty>No keys or indexes.</Empty>
  }

  return (
    <div className="flex flex-col gap-4 p-3 text-xs">
      {pk.length > 0 && (
        <Group title="Primary key">
          <code className="font-mono text-foreground">({pk.join(', ')})</code>
        </Group>
      )}
      {fks.length > 0 && (
        <Group title="Foreign keys">
          {fks.map((f) => (
            <div key={f.name} className="font-mono text-muted-foreground">
              <span className="text-foreground">{f.name}</span>: ({(f.columns ?? []).join(', ')}) →{' '}
              {f.references.namespace ? `${f.references.namespace}.` : ''}
              {f.references.name} ({(f.referenced_columns ?? []).join(', ')})
            </div>
          ))}
        </Group>
      )}
      {indexes.length > 0 && (
        <Group title="Indexes">
          {indexes.map((ix) => {
            const cols = ix.columns ?? []
            return (
              <div key={ix.name} className="font-mono text-muted-foreground">
                <span className="text-foreground">{ix.name}</span>
                {cols.length > 0 ? ` (${cols.join(', ')})` : ''}
                {ix.unique ? ' · unique' : ''}
              </div>
            )
          })}
        </Group>
      )}
    </div>
  )
}

function DescriptorList({ vm }: { vm: ObjectViewModel }) {
  const descriptors = (vm.detail.descriptors ?? []).filter((d) => d.kind !== 'source')
  if (descriptors.length === 0) {
    return <Empty>No details.</Empty>
  }
  return (
    <div className="flex flex-col gap-4 p-3 text-xs">
      {descriptors.map((d) => (
        <Group key={`${d.kind}:${d.title}`} title={d.title}>
          {(d.fields ?? []).map((f) => (
            <div key={f.name} className="flex gap-2">
              <span className="text-muted-foreground">{f.name}</span>
              <span className="font-mono text-foreground">{f.value}</span>
            </div>
          ))}
          {d.rows && (
            <table className="border-separate border-spacing-0">
              <thead>
                <tr>{(d.rows.columns ?? []).map((c) => <th key={c} className="border-b border-border px-2 py-1 text-left text-muted-foreground">{c}</th>)}</tr>
              </thead>
              <tbody>
                {(d.rows.rows ?? []).map((row, i) => (
                  <tr key={i}>{(row ?? []).map((v, j) => <td key={j} className="border-b border-border px-2 py-1 font-mono">{v}</td>)}</tr>
                ))}
              </tbody>
            </table>
          )}
        </Group>
      ))}
    </div>
  )
}

function Group({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-1">
      <div className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground">{title}</div>
      {children}
    </div>
  )
}

function Empty({ children }: { children: React.ReactNode }) {
  return <div className="p-4 text-xs text-muted-foreground">{children}</div>
}
