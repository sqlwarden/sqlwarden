import { useCallback, useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api } from '#/lib/api/client'
import { errorMessage, isApiError } from '#/lib/api/errors'
import { queryKeys } from '#/lib/api/query-keys'
import {
  connectionDetailQueryOptions,
  engineConnectionFieldsQueryOptions,
  revealConnectionSecret,
} from '#/lib/api/queries/workspace'
import type { Connection, ConnectionSecretName, ScopePath } from '#/lib/api/types'
import { driverMap, drivers } from './connection-drivers'
import {
  fieldDefaults,
  paramsFromValues,
  requiredFieldErrors,
  resolveFields,
} from './connection-drivers/resolveFields'
import { mapConnectionFieldErrors } from './connectionFormErrors'
import { emptySshState, type SshFormState } from './ConnectionSshFields'
import { emptyTlsState, type TlsFormState } from './ConnectionTlsFields'
import { applicableSecrets, sshRequestConfig, tlsRequestConfig } from './connectionConfigPayload'
import { sshConfigToState } from './connectionSshPayload'
import { tlsConfigToState } from './connectionTlsPayload'
import { findFrontendEngine } from './engines/registry'
import { useEngineNavigatorOptions } from './useEngineNavigatorOptions'
import { useSecretFields } from './useSecretFields'
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
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  orgSlug: string
  workspaceId: number
  connection: Connection | undefined
}) {
  const queryClient = useQueryClient()
  const driver = driverMap.get(connection?.driver ?? '') ?? drivers[0]
  const [name, setName] = useState('')
  const [edited, setFields] = useState<Record<string, string>>({})
  const [errors, setErrors] = useState<EditConnectionFormErrors>({ fields: {} })
  const [testState, setTestState] = useState<ConnectionTestState>({ status: 'idle' })
  const [conflict, setConflict] = useState(false)
  const [scopeDiscovery, setScopeDiscovery] = useState<ScopeDiscovery>()
  const [defaultScope, setDefaultScope] = useState<ScopePath>([])
  const [tls, setTls] = useState<TlsFormState>(emptyTlsState)
  const [ssh, setSsh] = useState<SshFormState>(emptySshState)
  const [showSystemSchemas, setShowSystemSchemas] = useState(false)
  const tlsSpec = findFrontendEngine(connection?.driver ?? '')?.tls
  const sshSupported = findFrontendEngine(connection?.driver ?? '')?.sshTunnel ?? false
  const [showAllDatabases, setShowAllDatabases] = useState(false)
  const { systemObjectsSupported, showAllDatabasesSupported } = useEngineNavigatorOptions(
    connection?.driver ?? '',
    open && connection !== undefined,
  )
  const showAllDatabasesForced = defaultScope.length === 0
  const effectiveShowAllDatabases = showAllDatabasesForced || showAllDatabases

  const active = open && connection !== undefined
  const detail = useQuery({
    ...connectionDetailQueryOptions(orgSlug, workspaceId, connection?.id ?? ''),
    enabled: active,
  })
  const fieldSpec = useQuery({
    ...engineConnectionFieldsQueryOptions(driver.id),
    enabled: active,
  })
  const resolvedFields = useMemo(
    () => resolveFields(fieldSpec.data ?? [], driver.fields),
    [fieldSpec.data, driver],
  )
  const fields = useMemo(
    () => ({ ...fieldDefaults(resolvedFields), ...edited }),
    [resolvedFields, edited],
  )

  const revealSecret = useCallback(
    async (secret: ConnectionSecretName) => {
      try {
        return await revealConnectionSecret(orgSlug, workspaceId, connection?.id ?? '', secret)
      } catch (error) {
        toast.error(errorMessage(error, 'Failed to reveal the saved value'))
        throw error
      }
    },
    [orgSlug, workspaceId, connection?.id],
  )
  const secrets = useSecretFields({ reveal: connection ? revealSecret : undefined })
  const { load: loadSecrets } = secrets

  useEffect(() => {
    if (!open || !connection) return
    setName(connection.name)
    setFields({})
    setErrors({ fields: {} })
    setTestState({ status: 'idle' })
    setConflict(false)
    setScopeDiscovery(undefined)
    setDefaultScope(connection.default_scope ?? [])
    setTls(emptyTlsState)
    setSsh(emptySshState)
    loadSecrets(undefined)
    setShowSystemSchemas(connection.show_system_schemas)
    setShowAllDatabases(connection.show_all_databases)
    // eslint-disable-next-line react-hooks/exhaustive-deps -- reset only when the dialog opens for a given connection
  }, [open, connection?.id])

  useEffect(() => {
    if (!open || !detail.data) return
    setFields(detail.data.params)
    setTls(tlsConfigToState(detail.data.tls_config))
    setSsh(sshConfigToState(detail.data.ssh_config))
    loadSecrets(detail.data.secrets)
  }, [open, detail.data, detail.dataUpdatedAt, loadSecrets])

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

  function changeShowAllDatabases(value: boolean) {
    setShowAllDatabases(value)
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

  function changeShowSystemSchemas(value: boolean) {
    setShowSystemSchemas(value)
  }

  function reset() {
    setName('')
    setFields({})
    setErrors({ fields: {} })
    setTestState({ status: 'idle' })
    setConflict(false)
    setScopeDiscovery(undefined)
    setDefaultScope([])
    setTls(emptyTlsState)
    setSsh(emptySshState)
    loadSecrets(undefined)
    setShowSystemSchemas(false)
    setShowAllDatabases(false)
    if (connection) {
      queryClient.removeQueries({
        queryKey: queryKeys.connectionDetail(orgSlug, workspaceId, connection.id),
      })
    }
  }

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) reset()
    onOpenChange(nextOpen)
  }

  function connectionPayload() {
    return {
      params: paramsFromValues(resolvedFields, fields),
      ...tlsRequestConfig(tls, {
        supported: Boolean(tlsSpec),
        stored: detail.data?.tls_config !== undefined,
      }),
      ...sshRequestConfig(ssh, {
        supported: sshSupported,
        stored: detail.data?.ssh_config !== undefined,
      }),
      secrets: applicableSecrets(secrets.payload, {
        tlsSupported: Boolean(tlsSpec),
        tls,
        sshSupported,
        ssh,
      }),
    }
  }

  function validate(): boolean {
    const nextErrors: EditConnectionFormErrors = {
      fields: requiredFieldErrors(resolvedFields, fields),
    }
    if (!name.trim()) nextErrors.name = 'Name is required.'
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
        ...connectionPayload(),
        driver: driver.id,
        connection_id: connection?.id,
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
        ...connectionPayload(),
        name: name.trim(),
        access_mode: connection?.access_mode ?? 'open',
        default_scope: defaultScope,
        show_system_schemas: showSystemSchemas,
        show_all_databases: effectiveShowAllDatabases,
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
        const mapped = mapConnectionFieldErrors(error.fieldErrors)
        setErrors({ name: mapped.name, fields: mapped.fields, _form: mapped.form })
        return
      }
      toast.error(errorMessage(error, 'Failed to update connection'))
    },
  })

  function submit(force = false) {
    if (!validate()) return
    void updateConnection.mutateAsync(force).catch(() => {})
  }

  const requiredFieldsFilled =
    fieldSpec.isSuccess &&
    detail.isSuccess &&
    Object.keys(requiredFieldErrors(resolvedFields, fields)).length === 0
  const loading = active && (detail.isPending || fieldSpec.isPending)
  const loadFailed = active && (detail.isError || fieldSpec.isError)

  return {
    bindSecret: secrets.bind,
    loading,
    loadFailed,
    resolvedFields,
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
    scopeDiscovery,
    selectDatabase,
    selectSchema,
    submit,
    testConnection,
    testState,
    tls,
    tlsSpec,
    changeTls,
    ssh,
    sshSupported,
    changeSsh,
    updateConnection,
    showSystemSchemas,
    systemObjectsSupported,
    showAllDatabases: effectiveShowAllDatabases,
    showAllDatabasesForced,
    showAllDatabasesSupported,
    changeShowAllDatabases,
    changeShowSystemSchemas,
  }
}
