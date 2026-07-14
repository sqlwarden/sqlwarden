import { useEffect, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api } from '#/lib/api/client'
import { errorMessage, isApiError } from '#/lib/api/errors'
import { queryKeys } from '#/lib/api/query-keys'
import type { Environment } from '#/lib/api/types'
import { defaultFieldValues, driverMap, drivers } from './connection-drivers'

export type ConnectionFormStage = 'driver' | 'form'
export type ConnectionTestState =
  | { status: 'idle' }
  | { status: 'pending' }
  | { status: 'ok'; latencyMs: number }
  | { status: 'error'; message: string }

export type ConnectionFormErrors = {
  name?: string
  environmentId?: string
  fields: Record<string, string>
  _form?: string
}

export function useConnectionForm({
  open,
  onOpenChange,
  orgSlug,
  workspaceId,
  environments,
  lockedEnvironmentId,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  orgSlug: string
  workspaceId: number
  environments: Environment[]
  lockedEnvironmentId?: number
}) {
  const queryClient = useQueryClient()
  const [stage, setStage] = useState<ConnectionFormStage>('driver')
  const [driverId, setDriverId] = useState(drivers[0].id)
  const [name, setName] = useState('')
  const [environmentId, setEnvironmentId] = useState('')
  const [fields, setFields] = useState<Record<string, string>>(() => defaultFieldValues(drivers[0]))
  const [errors, setErrors] = useState<ConnectionFormErrors>({ fields: {} })
  const [testState, setTestState] = useState<ConnectionTestState>({ status: 'idle' })
  const currentDriver = driverMap.get(driverId) ?? drivers[0]

  useEffect(() => {
    if (!open) return
    if (lockedEnvironmentId) {
      setEnvironmentId(String(lockedEnvironmentId))
    } else if (environments.length > 0 && !environmentId) {
      setEnvironmentId(String(environments[0].id))
    }
  }, [open, environments, environmentId, lockedEnvironmentId])

  function pickDriver(nextDriverId: string) {
    const definition = driverMap.get(nextDriverId)
    if (!definition) return
    if (nextDriverId !== driverId) {
      setFields(defaultFieldValues(definition))
      setErrors({ fields: {} })
      setTestState({ status: 'idle' })
    }
    setDriverId(nextDriverId)
    setStage('form')
  }

  function changeField(key: string, value: string) {
    setFields((current) => ({ ...current, [key]: value }))
    setErrors((current) => {
      const { [key]: _removed, ...remaining } = current.fields
      return { ...current, fields: remaining }
    })
    setTestState({ status: 'idle' })
  }

  function changeName(value: string) {
    setName(value)
    setErrors((current) => ({ ...current, name: undefined }))
  }

  function changeEnvironment(value: string) {
    setEnvironmentId(value)
    setErrors((current) => ({ ...current, environmentId: undefined }))
  }

  function reset() {
    setStage('driver')
    setDriverId(drivers[0].id)
    setName('')
    setEnvironmentId(
      lockedEnvironmentId
        ? String(lockedEnvironmentId)
        : environments.length > 0
          ? String(environments[0].id)
          : '',
    )
    setFields(defaultFieldValues(drivers[0]))
    setErrors({ fields: {} })
    setTestState({ status: 'idle' })
  }

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) reset()
    onOpenChange(nextOpen)
  }

  function buildDSN() {
    return currentDriver.buildDSN(fields)
  }

  function validate(): boolean {
    const nextErrors: ConnectionFormErrors = { fields: {} }
    if (!name.trim()) nextErrors.name = 'Name is required.'
    if (!environmentId) nextErrors.environmentId = 'Environment is required.'
    for (const field of currentDriver.fields) {
      if (field.required && !fields[field.key]?.trim()) {
        nextErrors.fields[field.key] = `${field.label} is required.`
      }
    }
    setErrors(nextErrors)
    return (
      !nextErrors.name && !nextErrors.environmentId && Object.keys(nextErrors.fields).length === 0
    )
  }

  const testConnection = useMutation({
    mutationFn: () =>
      api.post<{ ok: boolean; latency_ms: number; error?: string }>(
        `/api/v1/orgs/${orgSlug}/workspaces/${workspaceId}/connections/test`,
        { driver: driverId, dsn: buildDSN() },
      ),
    onMutate: () => setTestState({ status: 'pending' }),
    onSuccess: (data) =>
      setTestState(
        data.ok
          ? { status: 'ok', latencyMs: data.latency_ms }
          : { status: 'error', message: data.error ?? 'Connection failed.' },
      ),
    onError: () => setTestState({ status: 'error', message: 'Request failed.' }),
  })

  const createConnection = useMutation({
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
      reset()
      toast.success('Connection created')
      await queryClient.invalidateQueries({
        queryKey: queryKeys.orgWorkspaceConnectionsScope(orgSlug, workspaceId),
      })
    },
    onError: (error) => {
      if (isApiError(error) && error.fieldErrors) {
        const nextErrors: ConnectionFormErrors = { fields: {} }
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

  function submit() {
    if (stage !== 'form' || !validate()) return
    void createConnection.mutateAsync().catch(() => {})
  }

  const requiredFieldsFilled = currentDriver.fields
    .filter((field) => field.required)
    .every((field) => fields[field.key]?.trim())
  const selectedEnvironmentName =
    environments.find((environment) => String(environment.id) === environmentId)?.name ?? ''

  return {
    changeEnvironment,
    changeField,
    changeName,
    createConnection,
    currentDriver,
    driverId,
    environmentId,
    errors,
    fields,
    handleOpenChange,
    name,
    pickDriver,
    requiredFieldsFilled,
    selectedEnvironmentName,
    setStage,
    stage,
    submit,
    testConnection,
    testState,
  }
}
