import type { Connection, Environment } from '#/lib/api/types'
import { ALL_CONNECTIONS, ConnectionCombobox } from './ConnectionCombobox'
import { useIde } from './useIdeStore'

export { ALL_CONNECTIONS }
export type HistoryConnectionFilter = number | typeof ALL_CONNECTIONS

export function HistoryConnectionSelector({
  connections,
  environments,
  isLoading,
  value,
  activeHintConnectionId,
  onChange,
}: {
  connections: Connection[]
  environments: Environment[]
  isLoading: boolean
  value: HistoryConnectionFilter
  activeHintConnectionId?: number
  onChange: (value: HistoryConnectionFilter) => void
}) {
  const sessions = useIde((state) => state.sessions)

  return (
    <ConnectionCombobox
      connections={connections}
      environments={environments}
      sessions={sessions}
      isLoading={isLoading}
      value={value}
      activeHintConnectionId={activeHintConnectionId}
      includeAllOption
      placeholder={isLoading ? 'Loading connections…' : 'All connections'}
      ariaLabel="Filter by connection"
      align="start"
      triggerClassName="w-full justify-start"
      onSelectConnection={(connection) => onChange(connection.id)}
      onSelectAll={() => onChange(ALL_CONNECTIONS)}
    />
  )
}
