import { Skeleton } from '#/components/ui/skeleton'
import { permissionGroupName, type Permission } from '#/lib/permissions'
import type { PermissionDefinition } from '#/lib/api/types'

export function groupPermissions(
  permissions: readonly Permission[],
  definitions: ReadonlyMap<string, PermissionDefinition>,
) {
  const groups = new Map<string, Permission[]>()
  for (const item of permissions) {
    const group = permissionGroupName(item, definitions)
    groups.set(group, [...(groups.get(group) ?? []), item])
  }
  return Array.from(groups.entries()).map(([name, items]) => ({ name, permissions: items }))
}

export function PermissionGroupsSkeleton() {
  return (
    <div className="grid gap-6 sm:grid-cols-2">
      {Array.from({ length: 4 }).map((_, index) => (
        <div key={index} className="flex flex-col gap-3">
          <Skeleton className="h-3 w-24" />
          <div className="flex flex-col gap-3">
            <Skeleton className="h-4 w-40" />
            <Skeleton className="h-4 w-36" />
            <Skeleton className="h-4 w-44" />
          </div>
        </div>
      ))}
    </div>
  )
}
