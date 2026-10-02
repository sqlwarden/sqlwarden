import { createContext, useContext } from 'react'
import type { Connection } from '#/lib/api/types'

export type TabQueryRunner = (connection: Connection, sql: string) => Promise<void>

export type QueryRunRegistry = {
  register: (tabId: string, runner: TabQueryRunner) => void
  unregister: (tabId: string, runner: TabQueryRunner) => void
  get: (tabId: string) => TabQueryRunner | undefined
}

export function createQueryRunRegistry(): QueryRunRegistry {
  const runners = new Map<string, TabQueryRunner>()
  return {
    register: (tabId, runner) => runners.set(tabId, runner),
    unregister: (tabId, runner) => {
      if (runners.get(tabId) === runner) runners.delete(tabId)
    },
    get: (tabId) => runners.get(tabId),
  }
}

export const QueryRunRegistryContext = createContext<QueryRunRegistry>(createQueryRunRegistry())

export function useQueryRunRegistry(): QueryRunRegistry {
  return useContext(QueryRunRegistryContext)
}
