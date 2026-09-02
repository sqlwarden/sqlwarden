import { useState } from 'react'
import { Icon } from '#/lib/icons'
import type { Environment } from '#/lib/api/types'
import { cn } from '#/lib/utils'
import { Button } from '#/components/ui/button'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '#/components/ui/dialog'
import { Input } from '#/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '#/components/ui/select'
import { drivers, driverBrands } from './connection-drivers/index'
import { TestStatusIndicator } from './ConnectionTestStatus'
import { DriverFields, FormField } from './ConnectionFormFields'
import { DriverBadge } from './DriverBadge'
import { useConnectionForm } from './useConnectionForm'
import { useSetupStatus } from '#/hooks/use-setup-status'

// ─── Types ─────────────────────────────────────────────────────────────────────

// ─── Main component ─────────────────────────────────────────────────────────────

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  orgSlug: string
  workspaceId: number
  environments: Environment[]
  /** When set, the environment is pre-selected and the dropdown is locked. */
  lockedEnvironmentId?: number
  initialDriverId?: string
  initialFields?: Record<string, string>
  initialName?: string
}

export function ConnectionDialog({
  open,
  onOpenChange,
  orgSlug,
  workspaceId,
  environments,
  lockedEnvironmentId,
  initialDriverId,
  initialFields,
  initialName,
}: Props) {
  const setup = useSetupStatus()
  const availableDrivers = drivers.filter(
    (driver) => driver.id !== 'sqlite' || setup.data?.capabilities.local_sqlite_files === true,
  )
  const form = useConnectionForm({
    open,
    onOpenChange,
    orgSlug,
    workspaceId,
    environments,
    lockedEnvironmentId,
    initialDriverId,
    initialFields,
    initialName,
  })

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    form.submit()
  }
  const isPending = form.createConnection.isPending

  return (
    <Dialog open={open} onOpenChange={form.handleOpenChange}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>
            {form.stage === 'driver' ? 'Choose a database' : 'New Connection'}
          </DialogTitle>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="flex flex-col">
          <div className="flex max-h-[min(560px,calc(100svh-14rem))] flex-col gap-4 overflow-y-auto pb-1">
            {form.stage === 'driver' ? (
              <DriverGallery drivers={availableDrivers} onPick={form.pickDriver} />
            ) : (
              <>
                {/* Selected driver summary — one click back to the gallery. */}
                <div className="flex items-center gap-3 rounded-lg border border-border bg-muted/40 p-3">
                  <DriverBadge driver={form.driverId} size="md" className="size-8 shrink-0" />
                  <div className="min-w-0 flex-1">
                    <div className="text-xs font-medium text-foreground">
                      {form.currentDriver.label}
                    </div>
                    <div className="truncate text-[10px] text-muted-foreground">
                      {driverBrands[form.driverId]?.description}
                    </div>
                  </div>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    disabled={isPending}
                    onClick={() => form.setStage('driver')}
                  >
                    Change
                  </Button>
                </div>

                <div className="grid grid-cols-6 gap-3">
                  <div className={cn('col-span-6', environments.length > 0 && 'sm:col-span-3')}>
                    <FormField label="Name" error={form.errors.name}>
                      <Input
                        value={form.name}
                        disabled={isPending}
                        placeholder={`My ${form.currentDriver.label}`}
                        aria-invalid={form.errors.name ? true : undefined}
                        onChange={(e) => form.changeName(e.target.value)}
                      />
                    </FormField>
                  </div>

                  {environments.length > 0 ? (
                    <div className="col-span-6 sm:col-span-3">
                      <FormField label="Environment" error={form.errors.environmentId}>
                        <Select
                          value={form.environmentId}
                          onValueChange={(v) => {
                            if (!v) return
                            form.changeEnvironment(v)
                          }}
                          disabled={isPending || !!lockedEnvironmentId}
                        >
                          <SelectTrigger
                            aria-invalid={form.errors.environmentId ? true : undefined}
                            className="w-full"
                          >
                            <SelectValue>{form.selectedEnvironmentName}</SelectValue>
                          </SelectTrigger>
                          <SelectContent className="min-w-[180px]">
                            <SelectGroup>
                              {environments.map((env) => (
                                <SelectItem key={env.id} value={String(env.id)}>
                                  {env.name}
                                </SelectItem>
                              ))}
                            </SelectGroup>
                          </SelectContent>
                        </Select>
                      </FormField>
                    </div>
                  ) : null}

                  <DriverFields
                    driver={form.currentDriver}
                    values={form.fields}
                    errors={form.errors.fields}
                    disabled={isPending}
                    onChange={form.changeField}
                    scopeDiscovery={form.scopeDiscovery}
                    defaultScope={form.defaultScope}
                    onDatabaseChange={form.selectDatabase}
                    onSchemaChange={form.selectSchema}
                  />
                </div>

                {form.errors._form ? (
                  <p className="text-xs text-destructive">{form.errors._form}</p>
                ) : null}
              </>
            )}
          </div>

          <DialogFooter className="mt-4 items-center gap-3 border-t border-border/60 pt-4 sm:justify-between">
            {form.stage === 'form' ? (
              <div className="flex min-w-0 items-center gap-3">
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={
                    !form.requiredFieldsFilled || form.testConnection.isPending || isPending
                  }
                  onClick={() => void form.testConnection.mutateAsync().catch(() => {})}
                >
                  {form.testConnection.isPending ? 'Testing…' : 'Test Connection'}
                </Button>
                <TestStatusIndicator state={form.testState} />
              </div>
            ) : (
              <span />
            )}
            <div className="flex items-center gap-2">
              <DialogClose render={<Button type="button" variant="ghost" disabled={isPending} />}>
                Cancel
              </DialogClose>
              {form.stage === 'form' && (
                <Button type="submit" disabled={isPending}>
                  {isPending ? 'Creating…' : 'Create'}
                </Button>
              )}
            </div>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

// ─── Driver gallery ─────────────────────────────────────────────────────────────

function DriverGallery({
  drivers: availableDrivers,
  onPick,
}: {
  drivers: typeof drivers
  onPick: (driverId: string) => void
}) {
  const [search, setSearch] = useState('')
  const q = search.trim().toLowerCase()
  const filtered = availableDrivers.filter(
    (d) =>
      !q ||
      d.label.toLowerCase().includes(q) ||
      d.id.toLowerCase().includes(q) ||
      (driverBrands[d.id]?.description ?? '').toLowerCase().includes(q),
  )

  return (
    <div className="flex flex-col gap-3">
      <div className="relative">
        <Icon
          name="search-01"
          size={13}
          className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-muted-foreground"
        />
        <Input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search databases…"
          autoFocus
          className="h-8 pl-8"
        />
      </div>

      {filtered.length === 0 ? (
        <div className="rounded-lg border border-dashed border-border py-8 text-center text-xs text-muted-foreground">
          No databases match &ldquo;{search}&rdquo;
        </div>
      ) : (
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
          {filtered.map((d) => (
            <button
              key={d.id}
              type="button"
              onClick={() => onPick(d.id)}
              className={cn(
                'group flex flex-col items-start gap-2.5 rounded-xl border border-border bg-card p-3 text-left',
                'transition-all hover:border-primary/50 hover:bg-accent/40 hover:shadow-sm',
                'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
              )}
            >
              <DriverBadge driver={d.id} size="md" className="size-8" />
              <div className="min-w-0">
                <div className="text-xs font-medium text-foreground">{d.label}</div>
                <div className="mt-0.5 line-clamp-2 text-[10px] leading-relaxed text-muted-foreground">
                  {driverBrands[d.id]?.description}
                </div>
              </div>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
