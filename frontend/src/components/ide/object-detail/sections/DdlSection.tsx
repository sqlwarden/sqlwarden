import { sourceDescriptor } from '../baseRenderer'
import { ReadOnlySqlView } from '../ReadOnlySqlView'
import type { ObjectViewModel } from '../registry'

export function DdlSection({ vm }: { vm: ObjectViewModel }) {
  const body = sourceDescriptor(vm.detail, 'DDL') ?? sourceDescriptor(vm.detail, 'Definition')

  if (body === null) {
    return <div className="p-4 text-xs text-muted-foreground">No definition available.</div>
  }

  return (
    <div className="h-full min-h-0">
      <ReadOnlySqlView value={body} />
    </div>
  )
}
