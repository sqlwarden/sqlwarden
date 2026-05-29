import { useState, useEffect } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { HugeiconsIcon } from '@hugeicons/react'
import { Cancel01Icon, Tick02Icon } from '@hugeicons/core-free-icons'
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
import type { FieldDef } from './connection-drivers/index'

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

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  orgSlug: string
  workspaceId: number
  environments: Environment[]
}

export function ConnectionDialog({ open, onOpenChange, orgSlug, workspaceId, environments }: Props) {
  const queryClient = useQueryClient()

  const [driverId, setDriverId] = useState(drivers[0].id)
  const [name, setName] = useState('')
  const [environmentId, setEnvironmentId] = useState('')
  const [fields, setFields] = useState<Record<string, string>>(() => defaultFieldValues(drivers[0]))
  const [errors, setErrors] = useState<FormErrors>({ fields: {} })
  const [testState, setTestState] = useState<TestState>({ status: 'idle' })

  useEffect(() => {
    if (open && environments.length > 0 && !environmentId) {
      setEnvironmentId(String(environments[0].id))
    }
  }, [open, environments, environmentId])

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

  function handleNameChange(value: string) {
    setName(value)
    setErrors((prev) => ({ ...prev, name: undefined }))
    setTestState({ status: 'idle' })
  }

  function handleEnvironmentChange(value: string) {
    setEnvironmentId(value)
    setErrors((prev) => ({ ...prev, environmentId: undefined }))
  }

  function resetForm() {
    setDriverId(drivers[0].id)
    setName('')
    setEnvironmentId(environments.length > 0 ? String(environments[0].id) : '')
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

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>New Connection</DialogTitle>
        </DialogHeader>
        <form className="mt-6 flex flex-col gap-4" onSubmit={handleSubmit}>
          <div className="flex flex-col gap-2">
            <Label>Database</Label>
            <Select value={driverId} onValueChange={(v) => { if (v) handleDriverChange(v) }} disabled={isPending}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {drivers.map((d) => (
                  <SelectItem key={d.id} value={d.id}>
                    {d.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="flex flex-col gap-2">
            <Label>Name</Label>
            <Input
              value={name}
              disabled={isPending}
              placeholder="My Database"
              aria-invalid={errors.name ? true : undefined}
              onChange={(e) => handleNameChange(e.target.value)}
            />
            {errors.name ? <p className="text-xs text-destructive">{errors.name}</p> : null}
          </div>

          {environments.length > 0 ? (
            <div className="flex flex-col gap-2">
              <Label>Environment</Label>
              <Select value={environmentId} onValueChange={(v) => { if (v) handleEnvironmentChange(v) }} disabled={isPending}>
                <SelectTrigger aria-invalid={errors.environmentId ? true : undefined}>
                  <SelectValue placeholder="Select environment" />
                </SelectTrigger>
                <SelectContent>
                  {environments.map((env) => (
                    <SelectItem key={env.id} value={String(env.id)}>
                      {env.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {errors.environmentId ? <p className="text-xs text-destructive">{errors.environmentId}</p> : null}
            </div>
          ) : null}

          {currentDriver.fields.map((field) => (
            <ConnectionField
              key={`${driverId}-${field.key}`}
              field={field}
              value={fields[field.key] ?? ''}
              error={errors.fields[field.key]}
              disabled={isPending}
              onChange={(value) => handleFieldChange(field.key, value)}
            />
          ))}

          {errors._form ? <p className="text-xs text-destructive">{errors._form}</p> : null}

          <div className="flex items-center gap-3">
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

          <DialogFooter>
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

function ConnectionField({
  field,
  value,
  error,
  disabled,
  onChange,
}: {
  field: FieldDef
  value: string
  error?: string
  disabled: boolean
  onChange: (value: string) => void
}) {
  return (
    <div className="flex flex-col gap-2">
      <Label>{field.label}</Label>
      {field.type === 'select' ? (
        <Select value={value || (field.default ?? '')} onValueChange={(v) => { if (v) onChange(v) }} disabled={disabled}>
          <SelectTrigger aria-invalid={error ? true : undefined}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {(field.options ?? []).map((opt) => (
              <SelectItem key={opt.value} value={opt.value}>
                {opt.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      ) : (
        <Input
          type={field.type === 'password' ? 'password' : field.type === 'number' ? 'number' : 'text'}
          value={value}
          disabled={disabled}
          placeholder={field.placeholder}
          aria-invalid={error ? true : undefined}
          onChange={(e) => onChange(e.target.value)}
        />
      )}
      {error ? <p className="text-xs text-destructive">{error}</p> : null}
    </div>
  )
}

function TestStatusIndicator({ state }: { state: TestState }) {
  if (state.status === 'idle') return null
  if (state.status === 'pending') {
    return <span className="text-xs text-muted-foreground">Connecting…</span>
  }
  if (state.status === 'ok') {
    return (
      <span className="flex items-center gap-1 text-xs text-green-600 dark:text-green-400">
        <HugeiconsIcon icon={Tick02Icon} size={13} strokeWidth={2.5} />
        {state.latencyMs}ms
      </span>
    )
  }
  return (
    <span className="flex items-center gap-1 text-xs text-destructive">
      <HugeiconsIcon icon={Cancel01Icon} size={13} strokeWidth={2.5} />
      {state.message}
    </span>
  )
}
