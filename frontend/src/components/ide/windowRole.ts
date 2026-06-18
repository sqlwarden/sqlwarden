import { get, set, del } from 'idb-keyval'
import type { StateStorage } from 'zustand/middleware'

/**
 * 'managed' windows compete for the primary lock and persist only while holding it.
 * 'ephemeral' windows (future desktop "open in new window") never persist and never
 * claim primary — they seed a fresh, isolated state.
 */
export type WindowRole = 'managed' | 'ephemeral'

type IdbLike = {
  get: (k: string) => Promise<string | null | undefined>
  set: (k: string, v: string) => Promise<void>
  del: (k: string) => Promise<void>
}

const defaultIdb: IdbLike = { get: (k) => get<string>(k), set, del }

/** Storage that always reads but gates writes on canPersist() (primary status). */
export function makeRoleGatedStorage(
  key: string,
  canPersist: () => boolean,
  idb: IdbLike = defaultIdb,
): StateStorage {
  return {
    getItem: async () => (await idb.get(key)) ?? null,
    setItem: async (_name, value) => {
      if (canPersist()) await idb.set(key, value)
    },
    removeItem: async () => {
      if (canPersist()) await idb.del(key)
    },
  }
}

/**
 * Holds an exclusive Web Lock for as long as the window lives; the holder is the
 * single primary. When it releases (window closed / unmounted), a waiting window
 * is promoted. Returns a cleanup that releases the lock. Falls back to "always
 * primary" where the Web Locks API is unavailable (older browsers, jsdom).
 */
export function electPrimary(lockName: string, onBecamePrimary: () => void): () => void {
  const nav = globalThis.navigator as Navigator & { locks?: LockManager }
  if (!nav?.locks) {
    onBecamePrimary()
    return () => {}
  }
  const abort = new AbortController()
  let release: (() => void) | undefined
  nav.locks
    .request(lockName, { signal: abort.signal }, () =>
      new Promise<void>((resolve) => {
        release = resolve
        onBecamePrimary()
      }),
    )
    .catch(() => {}) // AbortError when we cancel a still-waiting request

  return () => {
    abort.abort() // cancel if still queued
    release?.() // release if we were holding it
  }
}
