export interface ExportRetryEntry {
  connectionId: number
  sql: string
  filename: string
  format: string
}

const cache = new Map<string, ExportRetryEntry>()

/** Session-scoped only (module-level Map, no persistence) — cleared on reload,
 *  matching the deliberate "cache miss after reload shows no Retry" behavior. */
export function rememberExportRetry(jobId: string, entry: ExportRetryEntry) {
  cache.set(jobId, entry)
}

export function getExportRetryEntry(jobId: string): ExportRetryEntry | undefined {
  return cache.get(jobId)
}
