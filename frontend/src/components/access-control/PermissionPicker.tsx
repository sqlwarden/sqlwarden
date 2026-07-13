import { useState } from 'react'
import { Icon } from '#/lib/icons'
import type { PermissionDefinition } from '#/lib/api/types'
import { permissionDescription, permissionDisplayName, type Permission } from '#/lib/permissions'
import { Checkbox } from '#/components/ui/checkbox'
import { Label } from '#/components/ui/label'
import { ScrollArea } from '#/components/ui/scroll-area'

export function groupPermissionDetails(permissions: readonly PermissionDefinition[]) {
  const groups = new Map<string, PermissionDefinition[]>()
  for (const item of permissions) groups.set(item.group, [...(groups.get(item.group) ?? []), item])
  return Array.from(groups.entries()).map(([name, items]) => ({ name, permissions: items }))
}

interface PermissionPickerProps {
  description: string
  idPrefix: string
  selectedPermissions: Set<Permission>
  permissionDetails: readonly PermissionDefinition[]
  permissionDefinitions: ReadonlyMap<string, PermissionDefinition>
  disabled: boolean
  error?: string
  onPermissionChecked: (value: Permission, checked: boolean) => void
}

export function PermissionPicker({
  description,
  idPrefix,
  selectedPermissions,
  permissionDetails,
  permissionDefinitions,
  disabled,
  error,
  onPermissionChecked,
}: PermissionPickerProps) {
  const [search, setSearch] = useState('')
  const groupedPermissions = groupPermissionDetails(permissionDetails)
  const query = search.toLowerCase()
  const filteredGroups = search
    ? groupedPermissions
        .map((group) => ({
          ...group,
          permissions: group.permissions.filter((item) =>
            item.label.toLowerCase().includes(query) ||
            item.description.toLowerCase().includes(query) ||
            item.key.toLowerCase().includes(query)),
        }))
        .filter((group) => group.permissions.length > 0)
    : groupedPermissions

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-end justify-between gap-4">
        <div className="flex flex-col gap-1">
          <Label>Permissions</Label>
          <p className="text-sm text-muted-foreground">{description}</p>
        </div>
        {selectedPermissions.size > 0 ? (
          <span className="shrink-0 text-xs text-muted-foreground">{selectedPermissions.size} of {permissionDetails.length} selected</span>
        ) : null}
      </div>
      <div className="rounded-md border border-border">
        <div className="flex items-center gap-2 border-b border-border px-3">
          <Icon name="search-01" size={20} className="size-4 shrink-0 text-muted-foreground" />
          <input
            type="text"
            placeholder="Filter permissions…"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            className="h-9 w-full bg-transparent text-sm outline-none placeholder:text-muted-foreground"
          />
          {search ? (
            <button type="button" onClick={() => setSearch('')} className="shrink-0 text-muted-foreground hover:text-foreground">
              <Icon name="cancel-01" size={20} className="size-3.5" />
            </button>
          ) : null}
        </div>
        <ScrollArea className="h-60">
          <div className="flex flex-col gap-5 p-4">
            {filteredGroups.length === 0 ? <p className="py-6 text-center text-sm text-muted-foreground">No permissions match your search.</p> : null}
            {filteredGroups.map((group) => (
              <div key={group.name} className="flex flex-col gap-3">
                <p className="text-[10px] font-semibold tracking-widest text-muted-foreground uppercase">{group.name}</p>
                <div className="flex flex-col gap-3">
                  {group.permissions.map((item) => {
                    const id = `${idPrefix}-${item.key.replace(/[^a-z0-9]+/g, '-')}`
                    const itemDescription = permissionDescription(item.key, permissionDefinitions)
                    return (
                      <label key={item.key} htmlFor={id} className="flex cursor-pointer items-start gap-2.5">
                        <Checkbox
                          id={id}
                          className="mt-0.5"
                          checked={selectedPermissions.has(item.key)}
                          disabled={disabled}
                          onCheckedChange={(checked) => onPermissionChecked(item.key, checked === true)}
                        />
                        <span className="flex flex-col gap-0.5">
                          <span className="text-sm font-medium text-foreground">{permissionDisplayName(item.key, permissionDefinitions)}</span>
                          {itemDescription ? <span className="text-xs text-muted-foreground">{itemDescription}</span> : null}
                        </span>
                      </label>
                    )
                  })}
                </div>
              </div>
            ))}
          </div>
        </ScrollArea>
      </div>
      {error ? <p className="text-sm text-destructive">{error}</p> : null}
    </div>
  )
}
