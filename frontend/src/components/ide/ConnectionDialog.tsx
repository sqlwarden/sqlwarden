import { errorMessage } from '#/lib/api/errors'
import { useState, useEffect } from 'react'
import { queryKeys } from '#/lib/api/query-keys'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Icon } from '#/lib/icons'
import { toast } from 'sonner'
import { api } from '#/lib/api/client'
import { isApiError } from '#/lib/api/errors'
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
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '#/components/ui/select'
import {
  drivers,
  driverMap,
  driverBrands,
  defaultFieldValues,
  type DriverDef,
  type FieldDef,
} from './connection-drivers/index'
import { DriverBadge } from './DriverBadge'

// ─── Types ─────────────────────────────────────────────────────────────────────

type Stage = 'driver' | 'form'

type TestState =
  | { status: 'idle' }
  | { status: 'pending' }
  | { status: 'ok'; latencyMs: number }
  | { status: 'error'; message: string }

type FormErrors = {
  name?: string
  environmentId?: string
  fields: Record<string, string>
  _form?: string
}

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

export function ConnectionDialog({ open, onOpenChange, orgSlug, workspaceId, environments, lockedEnvironmentId }: Props) {
  const queryClient = useQueryClient()

  const [stage, setStage] = useState<Stage>('driver')
  const [driverId, setDriverId] = useState(drivers[0].id)
  const [name, setName] = useState('')
  const [environmentId, setEnvironmentId] = useState('')
  const [fields, setFields] = useState<Record<string, string>>(() => defaultFieldValues(drivers[0]))
  const [errors, setErrors] = useState<FormErrors>({ fields: {} })
  const [testState, setTestState] = useState<TestState>({ status: 'idle' })

  useEffect(() => {
    if (!open) return
    if (lockedEnvironmentId) {
      setEnvironmentId(String(lockedEnvironmentId))
    } else if (environments.length > 0 && !environmentId) {
      setEnvironmentId(String(environments[0].id))
    }
  }, [open, environments, environmentId, lockedEnvironmentId])

  function handlePickDriver(newDriverId: string) {
    const def = driverMap.get(newDriverId)
    if (!def) return
    if (newDriverId !== driverId) {
      setFields(defaultFieldValues(def))
      setErrors({ fields: {} })
      setTestState({ status: 'idle' })
    }
    setDriverId(newDriverId)
    setStage('form')
  }

  function handleFieldChange(key: string, value: string) {
    setFields((prev) => ({ ...prev, [key]: value }))
    setErrors((prev) => {
      const { [key]: _removed, ...rest } = prev.fields
      return { ...prev, fields: rest }
    })
    setTestState({ status: 'idle' })
  }

  function resetForm() {
    setStage('driver')
    setDriverId(drivers[0].id)
    setName('')
    setEnvironmentId(
      lockedEnvironmentId
        ? String(lockedEnvironmentId)
        : environments.length > 0 ? String(environments[0].id) : '',
    )
    setFields(defaultFieldValues(drivers[0]))
    setErrors({ fields: {} })
    setTestState({ status: 'idle' })
  }

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) resetForm()
    onOpenChange(nextOpen)
  }

  const currentDriver = driverMap.get(driverId) ?? drivers[0]

  function buildDSN() {
    return currentDriver.buildDSN(fields)
  }

  function validateForm(): boolean {
    const nextErrors: FormErrors = { fields: {} }
    if (!name.trim()) nextErrors.name = 'Name is required.'
    if (!environmentId) nextErrors.environmentId = 'Environment is required.'
    for (const field of currentDriver.fields) {
      if (field.required && !fields[field.key]?.trim()) {
        nextErrors.fields[field.key] = `${field.label} is required.`
      }
    }
    setErrors(nextErrors)
    return !nextErrors.name && !nextErrors.environmentId && Object.keys(nextErrors.fields).length === 0
  }

  const testMutation = useMutation({
    mutationFn: () =>
      api.post<{ ok: boolean; latency_ms: number; error?: string }>(
        `/api/v1/orgs/${orgSlug}/workspaces/${workspaceId}/connections/test`,
        { driver: driverId, dsn: buildDSN() },
      ),
    onMutate: () => setTestState({ status: 'pending' }),
    onSuccess: (data) => {
      if (data.ok) {
        setTestState({ status: 'ok', latencyMs: data.latency_ms })
      } else {
        setTestState({ status: 'error', message: data.error ?? 'Connection failed.' })
      }
    },
    onError: () => setTestState({ status: 'error', message: 'Request failed.' }),
  })

  const createMutation = useMutation({
    mutationFn: () =>
      api.post(`/api/v1/orgs/${orgSlug}/workspaces/${workspaceId}/connections`, {
        name: name.trim(),
        driver: driverId,
        dsn: buildDSN(),
        environment_id: Number(environmentId),
        access_mode: 'open',
      }),
    onSuccess: async () => {
      onOpenChange(false)
      resetForm()
      toast.success('Connection created')
      await queryClient.invalidateQueries({ queryKey: queryKeys.orgWorkspaceConnectionsScope(orgSlug, workspaceId) })
    },
    onError: (error) => {
      if (isApiError(error) && error.fieldErrors) {
        const nextErrors: FormErrors = { fields: {} }
        if (error.fieldErrors.name) nextErrors.name = error.fieldErrors.name
        if (error.fieldErrors.driver || error.fieldErrors.dsn) {
          nextErrors._form = error.fieldErrors.driver ?? error.fieldErrors.dsn
        }
        setErrors(nextErrors)
        return
      }
      toast.error(errorMessage(error, 'Failed to create connection'))
    },
  })

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (stage !== 'form') return
    if (!validateForm()) return
    void createMutation.mutateAsync().catch(() => {})
  }

  const requiredFieldsFilled = currentDriver.fields
    .filter((f) => f.required)
    .every((f) => fields[f.key]?.trim())

  const isPending = createMutation.isPending
  const selectedEnvName = environments.find((e) => String(e.id) === environmentId)?.name ?? ''

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{stage === 'driver' ? 'Choose a database' : 'New Connection'}</DialogTitle>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="flex flex-col">
          <div className="flex max-h-[min(560px,calc(100svh-14rem))] flex-col gap-4 overflow-y-auto pb-1">
            {stage === 'driver' ? (
              <DriverGallery onPick={handlePickDriver} />
            ) : (
              <>
                {/* Selected driver summary — one click back to the gallery. */}
                <div className="flex items-center gap-3 rounded-lg border border-border bg-muted/40 p-3">
                  <DriverBadge driver={driverId} size="md" className="size-8 shrink-0" />
                  <div className="min-w-0 flex-1">
                    <div className="text-xs font-medium text-foreground">{currentDriver.label}</div>
                    <div className="truncate text-[10px] text-muted-foreground">
                      {driverBrands[driverId]?.description}
                    </div>
                  </div>
                  <Button type="button" variant="ghost" size="sm" disabled={isPending} onClick={() => setStage('driver')}>
                    Change
                  </Button>
                </div>

                <div className="grid grid-cols-6 gap-3">
                  <div className={cn('col-span-6', environments.length > 0 && 'sm:col-span-3')}>
                    <FormField label="Name" error={errors.name}>
                      <Input
                        value={name}
                        disabled={isPending}
                        placeholder={`My ${currentDriver.label}`}
                        aria-invalid={errors.name ? true : undefined}
                        onChange={(e) => {
                          setName(e.target.value)
                          setErrors((prev) => ({ ...prev, name: undefined }))
                        }}
                      />
                    </FormField>
                  </div>

                  {environments.length > 0 ? (
                    <div className="col-span-6 sm:col-span-3">
                      <FormField label="Environment" error={errors.environmentId}>
                        <Select
                          value={environmentId}
                          onValueChange={(v) => {
                            if (!v) return
                            setEnvironmentId(v)
                            setErrors((prev) => ({ ...prev, environmentId: undefined }))
                          }}
                          disabled={isPending || !!lockedEnvironmentId}
                        >
                          <SelectTrigger aria-invalid={errors.environmentId ? true : undefined} className="w-full">
                            <SelectValue>{selectedEnvName}</SelectValue>
                          </SelectTrigger>
                          <SelectContent className="min-w-[180px]">
                            {environments.map((env) => (
                              <SelectItem key={env.id} value={String(env.id)}>
                                {env.name}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </FormField>
                    </div>
                  ) : null}

                  <DriverFields
                    driver={currentDriver}
                    values={fields}
                    errors={errors.fields}
                    disabled={isPending}
                    onChange={handleFieldChange}
                  />
                </div>

                {errors._form ? <p className="text-xs text-destructive">{errors._form}</p> : null}
              </>
            )}
          </div>

          <DialogFooter className="mt-4 items-center gap-3 border-t border-border/60 pt-4 sm:justify-between">
            {stage === 'form' ? (
              <div className="flex min-w-0 items-center gap-3">
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={!requiredFieldsFilled || testMutation.isPending || isPending}
                  onClick={() => void testMutation.mutateAsync().catch(() => {})}
                >
                  {testMutation.isPending ? 'Testing…' : 'Test Connection'}
                </Button>
                <TestStatusIndicator state={testState} />
              </div>
            ) : (
              <span />
            )}
            <div className="flex items-center gap-2">
              <DialogClose render={<Button type="button" variant="ghost" disabled={isPending} />}>
                Cancel
              </DialogClose>
              {stage === 'form' && (
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
}: {
  driver: DriverDef
  values: Record<string, string>
  errors: Record<string, string>
  disabled: boolean
  onChange: (key: string, value: string) => void
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
    const selectedLabel = field.options?.find((o) => o.value === (value || field.default))?.label ?? value
    return (
      <Select
        value={value || field.default || ''}
        onValueChange={(v) => { if (v) onChange(field.key, v) }}
        disabled={disabled}
      >
        <SelectTrigger className="w-full">
          <SelectValue>{selectedLabel}</SelectValue>
        </SelectTrigger>
        <SelectContent className="min-w-[120px]">
          {(field.options ?? []).map((o) => (
            <SelectItem key={o.value} value={o.value}>
              {o.label}
            </SelectItem>
          ))}
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

function FormField({ label, error, children }: { label: string; error?: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-1.5">
      <Label>{label}</Label>
      {children}
      {error ? <p className="text-xs text-destructive">{error}</p> : null}
    </div>
  )
}

// ─── Test status indicator ───────────────────────────────────────────────────────

function TestStatusIndicator({ state }: { state: TestState }) {
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
      <span className="truncate" title={state.message}>{state.message}</span>
    </span>
  )
}
