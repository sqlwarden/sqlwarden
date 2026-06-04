import { useState, useEffect } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Icon } from '#/lib/icons'
import { driverBrands } from './connection-drivers/index'
import { toast } from 'sonner'
import { api } from '#/lib/api/client'
import { isApiError } from '#/lib/api/errors'
import type { Environment } from '#/lib/api/types'
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
import { drivers, driverMap, defaultFieldValues } from './connection-drivers/index'
import { DriverBadge } from './DriverBadge'

// ─── Types ─────────────────────────────────────────────────────────────────────

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

type DriverFormProps = {
  fields: Record<string, string>
  errors: Record<string, string>
  disabled: boolean
  onChange: (key: string, value: string) => void
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

  function handleDriverChange(newDriverId: string) {
    const def = driverMap.get(newDriverId)
    if (!def) return
    setDriverId(newDriverId)
    setFields(defaultFieldValues(def))
    setErrors({ fields: {} })
    setTestState({ status: 'idle' })
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
      await queryClient.invalidateQueries({ queryKey: ['org-workspace-connections', orgSlug, workspaceId] })
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
      toast.error(error instanceof Error ? error.message : 'Failed to create connection')
    },
  })

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
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
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>New Connection</DialogTitle>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="flex flex-col">
          <div className="flex flex-col gap-4 overflow-y-auto max-h-[min(560px,calc(100svh-14rem))] pb-1">

            {/* Driver selector */}
            <FormField label="Database">
              <Select
                value={driverId}
                onValueChange={(v) => { if (v) handleDriverChange(v) }}
                disabled={isPending}
              >
                <SelectTrigger>
                  <SelectValue>
                    <div className="flex items-center gap-2">
                      <DriverBadge driver={driverId} size="sm" />
                      <span>{currentDriver.label}</span>
                    </div>
                  </SelectValue>
                </SelectTrigger>
                <SelectContent className="min-w-[220px]">
                  {drivers.map((d) => (
                    <SelectItem key={d.id} value={d.id}>
                      <div className="flex items-center gap-2.5 py-0.5">
                        <DriverBadge driver={d.id} size="sm" />
                        <div>
                          <div className="text-xs font-medium">{d.label}</div>
                          <div className="text-[10px] text-muted-foreground">{driverBrands[d.id]?.description}</div>
                        </div>
                      </div>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </FormField>

            {/* Basic info */}
            <SectionDivider label="Details" />

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

            {environments.length > 0 ? (
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
                  <SelectTrigger aria-invalid={errors.environmentId ? true : undefined}>
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
            ) : null}

            {/* Per-driver connection form */}
            <SectionDivider label="Connection" />

            {driverId === 'postgres' ? (
              <PostgresForm
                fields={fields}
                errors={errors.fields}
                disabled={isPending}
                onChange={handleFieldChange}
              />
            ) : (
              <MysqlForm
                fields={fields}
                errors={errors.fields}
                disabled={isPending}
                onChange={handleFieldChange}
              />
            )}

            {errors._form ? <p className="text-xs text-destructive">{errors._form}</p> : null}

            {/* Test connection */}
            <div className="flex items-center gap-3 pt-1">
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

          </div>

          <DialogFooter className="mt-4 border-t border-border/60 pt-4">
            <DialogClose render={<Button type="button" variant="ghost" disabled={isPending} />}>
              Cancel
            </DialogClose>
            <Button type="submit" disabled={isPending}>
              {isPending ? 'Creating…' : 'Create'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

// ─── Per-driver form components ─────────────────────────────────────────────────

function PostgresForm({ fields, errors, disabled, onChange }: DriverFormProps) {
  const ssl = fields.sslmode || 'prefer'
  const sslLabel = { disable: 'Disable', prefer: 'Prefer', require: 'Require' }[ssl] ?? ssl

  return (
    <>
      <div className="grid grid-cols-[1fr_6.5rem] gap-3">
        <FormField label="Host" error={errors.host}>
          <Input
            value={fields.host ?? ''}
            placeholder="localhost"
            disabled={disabled}
            aria-invalid={errors.host ? true : undefined}
            onChange={(e) => onChange('host', e.target.value)}
          />
        </FormField>
        <FormField label="Port" error={errors.port}>
          <Input
            type="number"
            value={fields.port ?? '5432'}
            disabled={disabled}
            aria-invalid={errors.port ? true : undefined}
            onChange={(e) => onChange('port', e.target.value)}
          />
        </FormField>
      </div>

      <FormField label="Database" error={errors.database}>
        <Input
          value={fields.database ?? ''}
          placeholder="mydb"
          disabled={disabled}
          aria-invalid={errors.database ? true : undefined}
          onChange={(e) => onChange('database', e.target.value)}
        />
      </FormField>

      <SectionDivider label="Credentials" />

      <div className="grid grid-cols-2 gap-3">
        <FormField label="Username" error={errors.username}>
          <Input
            value={fields.username ?? ''}
            placeholder="postgres"
            disabled={disabled}
            aria-invalid={errors.username ? true : undefined}
            onChange={(e) => onChange('username', e.target.value)}
          />
        </FormField>
        <FormField label="Password">
          <Input
            type="password"
            value={fields.password ?? ''}
            disabled={disabled}
            onChange={(e) => onChange('password', e.target.value)}
          />
        </FormField>
      </div>

      <SectionDivider label="Security" />

      <FormField label="SSL Mode">
        <Select
          value={ssl}
          onValueChange={(v) => { if (v) onChange('sslmode', v) }}
          disabled={disabled}
        >
          <SelectTrigger>
            <SelectValue>{sslLabel}</SelectValue>
          </SelectTrigger>
          <SelectContent className="min-w-[120px]">
            <SelectItem value="disable">Disable</SelectItem>
            <SelectItem value="prefer">Prefer</SelectItem>
            <SelectItem value="require">Require</SelectItem>
          </SelectContent>
        </Select>
      </FormField>
    </>
  )
}

function MysqlForm({ fields, errors, disabled, onChange }: DriverFormProps) {
  return (
    <>
      <div className="grid grid-cols-[1fr_6.5rem] gap-3">
        <FormField label="Host" error={errors.host}>
          <Input
            value={fields.host ?? ''}
            placeholder="localhost"
            disabled={disabled}
            aria-invalid={errors.host ? true : undefined}
            onChange={(e) => onChange('host', e.target.value)}
          />
        </FormField>
        <FormField label="Port" error={errors.port}>
          <Input
            type="number"
            value={fields.port ?? '3306'}
            disabled={disabled}
            aria-invalid={errors.port ? true : undefined}
            onChange={(e) => onChange('port', e.target.value)}
          />
        </FormField>
      </div>

      <FormField label="Database" error={errors.database}>
        <Input
          value={fields.database ?? ''}
          placeholder="mydb"
          disabled={disabled}
          aria-invalid={errors.database ? true : undefined}
          onChange={(e) => onChange('database', e.target.value)}
        />
      </FormField>

      <SectionDivider label="Credentials" />

      <div className="grid grid-cols-2 gap-3">
        <FormField label="Username" error={errors.username}>
          <Input
            value={fields.username ?? ''}
            placeholder="root"
            disabled={disabled}
            aria-invalid={errors.username ? true : undefined}
            onChange={(e) => onChange('username', e.target.value)}
          />
        </FormField>
        <FormField label="Password">
          <Input
            type="password"
            value={fields.password ?? ''}
            disabled={disabled}
            onChange={(e) => onChange('password', e.target.value)}
          />
        </FormField>
      </div>
    </>
  )
}

// ─── Shared form helpers ────────────────────────────────────────────────────────

function SectionDivider({ label }: { label: string }) {
  return (
    <div className="flex items-center gap-2">
      <span className="shrink-0 text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
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
    <span className="flex items-center gap-1 text-xs text-destructive">
      <Icon name="cancel-01" size={13} />
      {state.message}
    </span>
  )
}
