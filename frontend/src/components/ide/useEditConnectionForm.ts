import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api } from '#/lib/api/client'
import { errorMessage, isApiError } from '#/lib/api/errors'
import { queryKeys } from '#/lib/api/query-keys'
import {
  connectionDsnQueryOptions,
  connectionSshQueryOptions,
  connectionTlsQueryOptions,
  orgQueryOptions,
} from '#/lib/api/query'
import type { Connection, ScopePath } from '#/lib/api/types'
import { defaultFieldValues, driverMap, drivers } from './connection-drivers'
import { emptySshState, type SshFormState } from './ConnectionSshFields'
import { emptyTlsState, type TlsFormState } from './ConnectionTlsFields'
import { sshRevealToState, sshStateToPayload } from './connectionSshPayload'
import { tlsRevealToState, tlsStateToPayload } from './connectionTlsPayload'
import { findFrontendEngine } from './engines/registry'
import {
  scopeSegmentName,
  type ConnectionTestState,
  type ScopeDiscovery,
} from './useConnectionForm'

export type EditConnectionFormErrors = {
  name?: string
  fields: Record<string, string>
  _form?: string
}

export function useEditConnectionForm({
  open,
  onOpenChange,
  orgSlug,
  workspaceId,
  connection,
  canRevealDsn,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  orgSlug: string
  workspaceId: number
  connection: Connection | undefined
  canRevealDsn: boolean
}) {
  const queryClient = useQueryClient()
  const driver = driverMap.get(connection?.driver ?? '') ?? drivers[0]
  const [name, setName] = useState('')
  const [fields, setFields] = useState<Record<string, string>>(() => defaultFieldValues(driver))
  const [errors, setErrors] = useState<EditConnectionFormErrors>({ fields: {} })
  const [testState, setTestState] = useState<ConnectionTestState>({ status: 'idle' })
  const [conflict, setConflict] = useState(false)
  const [scopeDiscovery, setScopeDiscovery] = useState<ScopeDiscovery>()
  const [defaultScope, setDefaultScope] = useState<ScopePath>([])
  const [tls, setTls] = useState<TlsFormState>(emptyTlsState)
  const [ssh, setSsh] = useState<SshFormState>(emptySshState)
  const tlsSpec = findFrontendEngine(connection?.driver ?? '')?.tls
  const sshSupported = findFrontendEngine(connection?.driver ?? '')?.sshTunnel ?? false

  const org = useQuery({ ...orgQueryOptions(orgSlug), enabled: open })
  const revealDsnAllowed =
    canRevealDsn && org.data !== undefined && !org.data.mask_connection_credentials_on_edit

  const revealDsn = useQuery({
    ...connectionDsnQueryOptions(orgSlug, workspaceId, connection?.id ?? ''),
    enabled: open && revealDsnAllowed && connection !== undefined,
  })

  const revealTls = useQuery({
    ...connectionTlsQueryOptions(orgSlug, workspaceId, connection?.id ?? ''),
    enabled: open && revealDsnAllowed && connection !== undefined,
  })

  const revealSsh = useQuery({
    ...connectionSshQueryOptions(orgSlug, workspaceId, connection?.id ?? ''),
    enabled: open && revealDsnAllowed && connection !== undefined,
  })

  useEffect(() => {
    if (!open || !connection) return
    setName(connection.name)
    setFields(defaultFieldValues(driver))
    setErrors({ fields: {} })
    setTestState({ status: 'idle' })
    setConflict(false)
    setScopeDiscovery(undefined)
    setDefaultScope(connection.default_scope ?? [])
    setTls(emptyTlsState)
    setSsh(emptySshState)
    // eslint-disable-next-line react-hooks/exhaustive-deps -- reset only when the dialog opens for a given connection
  }, [open, connection?.id])

  useEffect(() => {
    if (!open || !revealDsn.data) return
    setFields((current) => ({ ...current, ...driver.parseDSN(revealDsn.data.dsn) }))
    // eslint-disable-next-line react-hooks/exhaustive-deps -- re-apply on every successful fetch, even if the DSN value is unchanged (structural sharing keeps `data` referentially equal)
  }, [open, revealDsn.data, revealDsn.dataUpdatedAt])

  useEffect(() => {
    if (!open || !revealTls.data) return
    setTls(tlsRevealToState(revealTls.data))
  }, [open, revealTls.data, revealTls.dataUpdatedAt])

  useEffect(() => {
    if (!open || !revealSsh.data) return
    setSsh(sshRevealToState(revealSsh.data))
  }, [open, revealSsh.data, revealSsh.dataUpdatedAt])

  function changeField(key: string, value: string) {
    setFields((current) => ({ ...current, [key]: value }))
    setErrors((current) => {
      const { [key]: _removed, ...remaining } = current.fields
      return { ...current, fields: remaining }
    })
    setTestState({ status: 'idle' })
    setConflict(false)
    setScopeDiscovery(undefined)
    if (key === 'database') {
      setDefaultScope(value ? [{ kind: 'database', name: value }] : [])
    }
  }

  function changeName(value: string) {
    setName(value)
    setErrors((current) => ({ ...current, name: undefined }))
  }

  function changeTls(next: TlsFormState) {
    setTls(next)
    setTestState({ status: 'idle' })
    setConflict(false)
  }

  function changeSsh(next: SshFormState) {
    setSsh(next)
    setTestState({ status: 'idle' })
    setConflict(false)
  }

  function reset() {
    setName('')
    setFields(defaultFieldValues(driver))
    setErrors({ fields: {} })
    setTestState({ status: 'idle' })
    setConflict(false)
    setScopeDiscovery(undefined)
    setDefaultScope([])
    setTls(emptyTlsState)
    setSsh(emptySshState)
    if (connection) {
      queryClient.removeQueries({
        queryKey: queryKeys.connectionDsn(orgSlug, workspaceId, connection.id),
      })
      queryClient.removeQueries({
        queryKey: queryKeys.connectionTls(orgSlug, workspaceId, connection.id),
      })
      queryClient.removeQueries({
        queryKey: queryKeys.connectionSsh(orgSlug, workspaceId, connection.id),
      })
    }
  }

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) reset()
    onOpenChange(nextOpen)
  }

  function buildDSN() {
    return driver.buildDSN(fields)
  }

  function validate(): boolean {
    const nextErrors: EditConnectionFormErrors = { fields: {} }
    if (!name.trim()) nextErrors.name = 'Name is required.'
    for (const field of driver.fields) {
      if (field.required && !fields[field.key]?.trim()) {
        nextErrors.fields[field.key] = `${field.label} is required.`
      }
    }
    setErrors(nextErrors)
    return !nextErrors.name && Object.keys(nextErrors.fields).length === 0
  }

  const testConnection = useMutation({
    mutationFn: () =>
      api.post<{
        ok: boolean
        latency_ms: number
        error?: string
        scope_discovery?: ScopeDiscovery
        scope_discovery_error?: string
      }>(`/api/v1/orgs/${orgSlug}/workspaces/${workspaceId}/connections/test`, {
        driver: driver.id,
        dsn: buildDSN(),
        tls: tlsStateToPayload(tls),
        ...(ssh.enabled ? { ssh: sshStateToPayload(ssh) } : {}),
      }),
    onMutate: () => setTestState({ status: 'pending' }),
    onSuccess: (data) => {
      if (data.ok && data.scope_discovery) {
        setScopeDiscovery(data.scope_discovery)
        if (data.scope_discovery.current?.length) {
          const current = data.scope_discovery.current
          setDefaultScope(current)
          const database = scopeSegmentName(current, 'database')
          if (database) {
            setFields((values) => ({ ...values, database }))
          }
        }
      } else {
        setScopeDiscovery(undefined)
      }
      setTestState(
        data.ok
          ? {
              status: 'ok',
              latencyMs: data.latency_ms,
              discoveryError: data.scope_discovery_error,
            }
          : { status: 'error', message: data.error ?? 'Connection failed.' },
      )
    },
    onError: () => setTestState({ status: 'error', message: 'Request failed.' }),
  })

  function selectDatabase(database: string) {
    if (!database) {
      setFields((values) => ({ ...values, database: '' }))
      setDefaultScope([])
      setTestState({ status: 'idle' })
      return
    }
    const databaseScope = scopeDiscovery?.scopes.find(
      (scope) => scope.length === 1 && scopeSegmentName(scope, 'database') === database,
    ) ?? [{ kind: 'database', name: database }]
    setFields((values) => ({ ...values, database }))
    const current = scopeDiscovery?.current
    if (current && scopeSegmentName(current, 'database') !== database) {
      setTestState({ status: 'idle' })
    }
    setDefaultScope(
      current && scopeSegmentName(current, 'database') === database ? current : databaseScope,
    )
  }

  function selectSchema(schema: string) {
    const database = scopeSegmentName(defaultScope, 'database') ?? fields.database
    if (!schema) {
      setDefaultScope(database ? [{ kind: 'database', name: database }] : [])
      return
    }
    const discovered = scopeDiscovery?.scopes.find(
      (scope) =>
        scopeSegmentName(scope, 'database') === database &&
        scopeSegmentName(scope, 'schema') === schema,
    )
    setDefaultScope(
      discovered ?? [
        { kind: 'database', name: database },
        { kind: 'schema', name: schema },
      ],
    )
  }

  const updateConnection = useMutation({
    mutationFn: (force: boolean) =>
      api.patch(`/api/v1/orgs/${orgSlug}/workspaces/${workspaceId}/connections/${connection?.id}`, {
        name: name.trim(),
        dsn: buildDSN(),
        access_mode: connection?.access_mode ?? 'open',
        default_scope: defaultScope,
        tls: tlsStateToPayload(tls),
        ssh: sshStateToPayload(ssh),
        force,
      }),
    onSuccess: async () => {
      onOpenChange(false)
      reset()
      toast.success('Connection updated')
      await queryClient.invalidateQueries({
        queryKey: queryKeys.orgWorkspaceConnectionsScope(orgSlug, workspaceId),
      })
    },
    onError: (error) => {
      if (isApiError(error) && error.status === 409) {
        setConflict(true)
        return
      }
      if (isApiError(error) && error.fieldErrors) {
        const nextErrors: EditConnectionFormErrors = { fields: {} }
        if (error.fieldErrors.name) nextErrors.name = error.fieldErrors.name
        if (error.fieldErrors.dsn) nextErrors._form = error.fieldErrors.dsn
        setErrors(nextErrors)
        return
      }
      toast.error(errorMessage(error, 'Failed to update connection'))
    },
  })

  const removeTls = useMutation({
    mutationFn: () =>
      api.delete(
        `/api/v1/orgs/${orgSlug}/workspaces/${workspaceId}/connections/${connection?.id}/tls`,
      ),
    onSuccess: async () => {
      setTls(emptyTlsState)
      setConflict(false)
      toast.success('TLS configuration removed')
      if (connection) {
        await queryClient.invalidateQueries({
          queryKey: queryKeys.connectionTls(orgSlug, workspaceId, connection.id),
        })
      }
      await queryClient.invalidateQueries({
        queryKey: queryKeys.orgWorkspaceConnectionsScope(orgSlug, workspaceId),
      })
    },
    onError: (error) => toast.error(errorMessage(error, 'Failed to remove TLS configuration')),
  })

  const removeSsh = useMutation({
    mutationFn: () =>
      api.delete(
        `/api/v1/orgs/${orgSlug}/workspaces/${workspaceId}/connections/${connection?.id}/ssh`,
      ),
    onSuccess: async () => {
      setSsh(emptySshState)
      setConflict(false)
      toast.success('SSH configuration removed')
      if (connection) {
        await queryClient.invalidateQueries({
          queryKey: queryKeys.connectionSsh(orgSlug, workspaceId, connection.id),
        })
      }
      await queryClient.invalidateQueries({
        queryKey: queryKeys.orgWorkspaceConnectionsScope(orgSlug, workspaceId),
      })
    },
    onError: (error) => toast.error(errorMessage(error, 'Failed to remove SSH configuration')),
  })

  function submit(force = false) {
    if (!validate()) return
    void updateConnection.mutateAsync(force).catch(() => {})
  }

  const requiredFieldsFilled = driver.fields
    .filter((field) => field.required)
    .every((field) => fields[field.key]?.trim())

  return {
    changeField,
    changeName,
    conflict,
    defaultScope,
    driver,
    errors,
    fields,
    handleOpenChange,
    name,
    requiredFieldsFilled,
    revealDsnAllowed,
    revealDsnPending: revealDsn.isFetching,
    revealTlsPending: revealTls.isFetching,
    revealSshPending: revealSsh.isFetching,
    scopeDiscovery,
    selectDatabase,
    selectSchema,
    submit,
    testConnection,
    testState,
    tls,
    tlsSpec,
    tlsConfigured: revealTls.data?.configured ?? false,
    changeTls,
    removeTls,
    ssh,
    sshSupported,
    sshConfigured: revealSsh.data?.configured ?? false,
    changeSsh,
    removeSsh,
    updateConnection,
  }
}
