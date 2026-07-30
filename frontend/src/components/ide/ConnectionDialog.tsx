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
import { Label } from '#/components/ui/label'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '#/components/ui/select'
import { drivers, driverBrands, type DriverDef, type FieldDef } from './connection-drivers/index'
import { DriverBadge } from './DriverBadge'
import {
  scopeSegmentName,
  useConnectionForm,
  type ConnectionTestState,
  type ScopeDiscovery,
} from './useConnectionForm'

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
}

const NO_DEFAULT_SCOPE = '__sqlwarden_no_default_scope__'

export function ConnectionDialog({
  open,
  onOpenChange,
  orgSlug,
  workspaceId,
  environments,
  lockedEnvironmentId,
}: Props) {
  const form = useConnectionForm({
    open,
    onOpenChange,
    orgSlug,
    workspaceId,
    environments,
    lockedEnvironmentId,
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
              <DriverGallery onPick={form.pickDriver} />
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

function DriverGallery({ onPick }: { onPick: (driverId: string) => void }) {
  const [search, setSearch] = useState('')
  const q = search.trim().toLowerCase()
  const filtered = drivers.filter(
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

// ─── Registry-driven driver form ─────────────────────────────────────────────────
// Fields, sections, and layout all come from the driver definition, so adding a
// new database means writing one driver def — no dialog changes.

const SPAN_CLASS: Record<NonNullable<FieldDef['span']>, string> = {
  full: 'col-span-6',
  wide: 'col-span-4',
  half: 'col-span-3',
  compact: 'col-span-2',
}

function DriverFields({
  driver,
  values,
  errors,
  disabled,
  onChange,
  scopeDiscovery,
  defaultScope,
  onDatabaseChange,
  onSchemaChange,
}: {
  driver: DriverDef
  values: Record<string, string>
  errors: Record<string, string>
  disabled: boolean
  onChange: (key: string, value: string) => void
  scopeDiscovery?: ScopeDiscovery
  defaultScope: { kind: string; name: string }[]
  onDatabaseChange: (database: string) => void
  onSchemaChange: (schema: string) => void
}) {
  const nodes: React.ReactNode[] = []
  let lastSection: string | undefined

  for (const field of driver.fields) {
    if (field.section && field.section !== lastSection) {
      lastSection = field.section
      nodes.push(
        <div key={`section:${field.section}`} className="col-span-6">
          <SectionDivider label={field.section} />
        </div>,
      )
    }
    if (field.key === 'database' && scopeDiscovery && discoveredDatabases(scopeDiscovery).length) {
      nodes.push(
        <ScopeFields
          key="database"
          discovery={scopeDiscovery}
          defaultScope={defaultScope}
          disabled={disabled}
          onDatabaseChange={onDatabaseChange}
          onSchemaChange={onSchemaChange}
        />,
      )
      continue
    }
    nodes.push(
      <div key={field.key} className={SPAN_CLASS[field.span ?? 'full']}>
        <FormField label={field.label} error={errors[field.key]}>
          <DriverFieldControl
            field={field}
            value={values[field.key] ?? ''}
            invalid={Boolean(errors[field.key])}
            disabled={disabled}
            onChange={onChange}
          />
        </FormField>
      </div>,
    )
  }

  return <>{nodes}</>
}

function ScopeFields({
  discovery,
  defaultScope,
  disabled,
  onDatabaseChange,
  onSchemaChange,
}: {
  discovery: ScopeDiscovery
  defaultScope: { kind: string; name: string }[]
  disabled: boolean
  onDatabaseChange: (database: string) => void
  onSchemaChange: (schema: string) => void
}) {
  const database = scopeSegmentName(defaultScope, 'database') ?? ''
  const schema = scopeSegmentName(defaultScope, 'schema') ?? ''
  const databases = discoveredDatabases(discovery)
  const schemas = uniqueNames(
    discovery.scopes
      .filter((scope) => scopeSegmentName(scope, 'database') === database)
      .map((scope) => scopeSegmentName(scope, 'schema')),
  )

  return (
    <>
      <div className={cn('col-span-6', schemas.length > 0 && 'sm:col-span-3')}>
        <FormField label="Database">
          <Select
            value={database || NO_DEFAULT_SCOPE}
            onValueChange={(value) => {
              if (value) onDatabaseChange(value === NO_DEFAULT_SCOPE ? '' : value)
            }}
            disabled={disabled}
          >
            <SelectTrigger className="w-full" aria-label="Default database">
              <SelectValue>{database || 'No default database'}</SelectValue>
            </SelectTrigger>
            <SelectContent className="min-w-[180px]">
              <SelectGroup>
                <SelectItem value={NO_DEFAULT_SCOPE}>No default database</SelectItem>
                {databases.map((name) => (
                  <SelectItem key={name} value={name}>
                    {name}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </FormField>
      </div>
      {schemas.length > 0 ? (
        <div className="col-span-6 sm:col-span-3">
          <FormField label="Schema">
            <Select
              value={schema || NO_DEFAULT_SCOPE}
              onValueChange={(value) => {
                if (value) onSchemaChange(value === NO_DEFAULT_SCOPE ? '' : value)
              }}
              disabled={disabled}
            >
              <SelectTrigger className="w-full" aria-label="Default schema">
                <SelectValue>{schema || 'Use database default'}</SelectValue>
              </SelectTrigger>
              <SelectContent className="min-w-[180px]">
                <SelectGroup>
                  <SelectItem value={NO_DEFAULT_SCOPE}>Use database default</SelectItem>
                  {schemas.map((name) => (
                    <SelectItem key={name} value={name}>
                      {name}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </FormField>
        </div>
      ) : null}
    </>
  )
}

function discoveredDatabases(discovery: ScopeDiscovery): string[] {
  return uniqueNames([
    ...discovery.scopes
      .filter((scope) => scope.length === 1)
      .map((scope) => scopeSegmentName(scope, 'database')),
    scopeSegmentName(discovery.current, 'database'),
  ])
}

function uniqueNames(names: (string | undefined)[]): string[] {
  return [...new Set(names.filter((name): name is string => Boolean(name)))].sort((a, b) =>
    a.localeCompare(b),
  )
}

function DriverFieldControl({
  field,
  value,
  invalid,
  disabled,
  onChange,
}: {
  field: FieldDef
  value: string
  invalid: boolean
  disabled: boolean
  onChange: (key: string, value: string) => void
}) {
  if (field.type === 'select') {
    const selectedLabel =
      field.options?.find((o) => o.value === (value || field.default))?.label ?? value
    return (
      <Select
        value={value || field.default || ''}
        onValueChange={(v) => {
          if (v) onChange(field.key, v)
        }}
        disabled={disabled}
      >
        <SelectTrigger className="w-full">
          <SelectValue>{selectedLabel}</SelectValue>
        </SelectTrigger>
        <SelectContent className="min-w-[120px]">
          <SelectGroup>
            {(field.options ?? []).map((o) => (
              <SelectItem key={o.value} value={o.value}>
                {o.label}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    )
  }
  return (
    <Input
      type={field.type === 'number' ? 'number' : field.type === 'password' ? 'password' : 'text'}
      value={value}
      placeholder={field.placeholder}
      disabled={disabled}
      aria-invalid={invalid ? true : undefined}
      onChange={(e) => onChange(field.key, e.target.value)}
    />
  )
}

// ─── Shared form helpers ────────────────────────────────────────────────────────

function SectionDivider({ label }: { label: string }) {
  return (
    <div className="flex items-center gap-2 pt-1">
      <span className="shrink-0 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
        {label}
      </span>
      <div className="h-px flex-1 bg-border" />
    </div>
  )
}

function FormField({
  label,
  error,
  children,
}: {
  label: string
  error?: string
  children: React.ReactNode
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <Label>{label}</Label>
      {children}
      {error ? <p className="text-xs text-destructive">{error}</p> : null}
    </div>
  )
}

// ─── Test status indicator ───────────────────────────────────────────────────────

function TestStatusIndicator({ state }: { state: ConnectionTestState }) {
  if (state.status === 'idle') return null
  if (state.status === 'pending') {
    return <span className="text-xs text-muted-foreground">Connecting…</span>
  }
  if (state.status === 'ok') {
    return (
      <span className="flex items-center gap-1 text-xs text-green-600 dark:text-green-400">
        <Icon name="tick-02" size={13} />
        {state.latencyMs}ms
      </span>
    )
  }
  return (
    <span className="flex min-w-0 items-center gap-1 text-xs text-destructive">
      <Icon name="cancel-01" size={13} className="shrink-0" />
      <span className="truncate" title={state.message}>
        {state.message}
      </span>
    </span>
  )
}
