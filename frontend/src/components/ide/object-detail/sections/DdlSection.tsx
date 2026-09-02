import { useQuery } from '@tanstack/react-query'
import { orgConnectionObjectDefinitionQueryOptions } from '#/lib/api/queries/database'
import { sourceDescriptor } from '../baseRenderer'
import { ReadOnlySqlView } from '../ReadOnlySqlView'
import type { ObjectViewModel } from '../registry'

export function DdlSection({ vm }: { vm: ObjectViewModel }) {
  // pg/mysql/sqlite embed the definition in bulk object inspection. Engines that
  // omit it for cost reasons (Oracle) leave no inline descriptor, so fetch it
  // on demand only when this section is actually opened.
  const inline = sourceDescriptor(vm.detail, 'DDL') ?? sourceDescriptor(vm.detail, 'Definition')

  const lazy = useQuery(
    orgConnectionObjectDefinitionQueryOptions(
      vm.orgSlug,
      vm.workspaceId,
      vm.connectionId,
      vm.sessionId || undefined,
      vm.detail.ref,
      inline === null,
    ),
  )

  const body = inline ?? lazy.data?.source?.body ?? null

  if (body === null) {
    if (inline === null && lazy.isLoading) {
      return <div className="p-4 text-xs text-muted-foreground">Loading definition…</div>
    }
    if (inline === null && lazy.isError) {
      return <div className="p-4 text-xs text-muted-foreground">Could not load definition.</div>
    }
    return <div className="p-4 text-xs text-muted-foreground">No definition available.</div>
  }
  return (
    <div className="h-full min-h-0">
      <ReadOnlySqlView value={body} />
    </div>
  )
}
