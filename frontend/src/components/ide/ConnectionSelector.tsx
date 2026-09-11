import type { Connection, Environment } from '#/lib/api/types'
import { ConnectionCombobox } from './ConnectionCombobox'
import { useIde } from './useIdeStore'

export function ConnectionSelector({
  activeConnection,
  activeConnectionId,
  connections,
  environments,
  isLoading,
  tabAvailable,
  onSelect,
}: {
  activeConnection?: Connection
  activeConnectionId?: number
  connections: Connection[]
  environments: Environment[]
  isLoading: boolean
  tabAvailable: boolean
  onSelect: (connection: Connection) => void
}) {
  const sessions = useIde((state) => state.sessions)

  return (
    <ConnectionCombobox
      connections={connections}
      environments={environments}
      sessions={sessions}
      isLoading={isLoading}
      disabled={!tabAvailable}
      value={activeConnectionId ?? activeConnection?.id}
      placeholder={isLoading ? 'Loading connections…' : 'Select connection…'}
      ariaLabel="Select connection"
      tipLabel={
        activeConnection
          ? sessions[activeConnection.id]
            ? `Connected to ${activeConnection.name}`
            : 'Not connected — Run connects automatically'
          : 'Choose a connection to run queries'
      }
      align="end"
      onSelectConnection={onSelect}
    />
  )
}
