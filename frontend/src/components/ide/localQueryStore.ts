const DB_NAME = 'sqlwarden-query-store'
const DB_VERSION = 1
const HISTORY_STORE = 'history'
const FAVORITES_STORE = 'favorites'

export interface LocalHistoryEntry {
  id: string
  connectionId: number
  sqlText: string
  status: 'ok' | 'error' | 'cancelled'
  errorMessage?: string
  durationMs: number
  rowsAffected: number
  executedAt: string
}

export interface LocalFavorite {
  id: string
  workspaceId: number
  connectionId: number | null
  name: string
  sqlText: string
  createdAt: string
  updatedAt: string
}

export function openLocalQueryDB(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(DB_NAME, DB_VERSION)
    request.onupgradeneeded = () => {
      const db = request.result
      if (!db.objectStoreNames.contains(HISTORY_STORE)) {
        const store = db.createObjectStore(HISTORY_STORE, { keyPath: 'id' })
        store.createIndex('connectionId', 'connectionId')
      }
      if (!db.objectStoreNames.contains(FAVORITES_STORE)) {
        const store = db.createObjectStore(FAVORITES_STORE, { keyPath: 'id' })
        store.createIndex('workspaceId', 'workspaceId')
      }
    }
    request.onsuccess = () => resolve(request.result)
    request.onerror = () => reject(request.error)
  })
}

function randomId(): string {
  return crypto.randomUUID()
}

export async function addLocalHistoryEntry(
  entry: Omit<LocalHistoryEntry, 'id' | 'executedAt'>,
  retentionCount: number,
): Promise<LocalHistoryEntry> {
  const db = await openLocalQueryDB()
  const full: LocalHistoryEntry = { ...entry, id: randomId(), executedAt: new Date().toISOString() }

  await new Promise<void>((resolve, reject) => {
    const tx = db.transaction(HISTORY_STORE, 'readwrite')
    tx.objectStore(HISTORY_STORE).add(full)
    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })

  const all = await listLocalHistory(entry.connectionId)
  if (all.length > retentionCount) {
    const toRemove = all.slice(retentionCount)
    await new Promise<void>((resolve, reject) => {
      const tx = db.transaction(HISTORY_STORE, 'readwrite')
      const store = tx.objectStore(HISTORY_STORE)
      for (const stale of toRemove) store.delete(stale.id)
      tx.oncomplete = () => resolve()
      tx.onerror = () => reject(tx.error)
    })
  }

  return full
}

export async function listLocalHistory(connectionId: number): Promise<LocalHistoryEntry[]> {
  const db = await openLocalQueryDB()
  return new Promise((resolve, reject) => {
    const tx = db.transaction(HISTORY_STORE, 'readonly')
    const index = tx.objectStore(HISTORY_STORE).index('connectionId')
    const request = index.getAll(connectionId)
    request.onsuccess = () => {
      const entries = (request.result as LocalHistoryEntry[]).sort((a, b) =>
        b.executedAt.localeCompare(a.executedAt),
      )
      resolve(entries)
    }
    request.onerror = () => reject(request.error)
  })
}

export interface LocalHistoryPage {
  items: LocalHistoryEntry[]
  page: number
  page_size: number
  total: number
}

export async function listLocalHistoryPage(params: {
  connectionId?: number
  search?: string
  page: number
  pageSize: number
}): Promise<LocalHistoryPage> {
  const { connectionId, search, page, pageSize } = params
  const db = await openLocalQueryDB()
  const all = await new Promise<LocalHistoryEntry[]>((resolve, reject) => {
    const tx = db.transaction(HISTORY_STORE, 'readonly')
    const store = tx.objectStore(HISTORY_STORE)
    const request =
      connectionId === undefined ? store.getAll() : store.index('connectionId').getAll(connectionId)
    request.onsuccess = () => resolve(request.result as LocalHistoryEntry[])
    request.onerror = () => reject(request.error)
  })

  const query = search?.trim().toLowerCase()
  const matching = query ? all.filter((entry) => entry.sqlText.toLowerCase().includes(query)) : all
  const sorted = matching.sort((a, b) => b.executedAt.localeCompare(a.executedAt))
  const start = (page - 1) * pageSize
  return {
    items: sorted.slice(start, start + pageSize),
    page,
    page_size: pageSize,
    total: sorted.length,
  }
}

export async function clearLocalHistory(connectionId: number): Promise<void> {
  const db = await openLocalQueryDB()
  const entries = await listLocalHistory(connectionId)
  await new Promise<void>((resolve, reject) => {
    const tx = db.transaction(HISTORY_STORE, 'readwrite')
    const store = tx.objectStore(HISTORY_STORE)
    for (const entry of entries) store.delete(entry.id)
    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })
}

export async function addLocalFavorite(
  favorite: Omit<LocalFavorite, 'id' | 'createdAt' | 'updatedAt'>,
): Promise<LocalFavorite> {
  const db = await openLocalQueryDB()
  const now = new Date().toISOString()
  const full: LocalFavorite = { ...favorite, id: randomId(), createdAt: now, updatedAt: now }

  await new Promise<void>((resolve, reject) => {
    const tx = db.transaction(FAVORITES_STORE, 'readwrite')
    tx.objectStore(FAVORITES_STORE).add(full)
    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })

  return full
}

export async function listLocalFavorites(workspaceId: number): Promise<LocalFavorite[]> {
  const db = await openLocalQueryDB()
  return new Promise((resolve, reject) => {
    const tx = db.transaction(FAVORITES_STORE, 'readonly')
    const index = tx.objectStore(FAVORITES_STORE).index('workspaceId')
    const request = index.getAll(workspaceId)
    request.onsuccess = () => {
      const entries = (request.result as LocalFavorite[]).sort((a, b) =>
        a.name.localeCompare(b.name),
      )
      resolve(entries)
    }
    request.onerror = () => reject(request.error)
  })
}

export async function updateLocalFavorite(
  id: string,
  patch: { name: string; sqlText: string },
): Promise<LocalFavorite | null> {
  const db = await openLocalQueryDB()
  return new Promise((resolve, reject) => {
    const tx = db.transaction(FAVORITES_STORE, 'readwrite')
    const store = tx.objectStore(FAVORITES_STORE)
    const getRequest = store.get(id)
    getRequest.onsuccess = () => {
      const existing = getRequest.result as LocalFavorite | undefined
      if (!existing) {
        resolve(null)
        return
      }
      const updated: LocalFavorite = {
        ...existing,
        name: patch.name,
        sqlText: patch.sqlText,
        updatedAt: new Date().toISOString(),
      }
      store.put(updated)
      tx.oncomplete = () => resolve(updated)
    }
    getRequest.onerror = () => reject(getRequest.error)
    tx.onerror = () => reject(tx.error)
  })
}

export async function deleteLocalFavorite(id: string): Promise<void> {
  const db = await openLocalQueryDB()
  await new Promise<void>((resolve, reject) => {
    const tx = db.transaction(FAVORITES_STORE, 'readwrite')
    tx.objectStore(FAVORITES_STORE).delete(id)
    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })
}
