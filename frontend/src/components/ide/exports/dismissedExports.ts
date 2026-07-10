const KEY_PREFIX = 'sqlwarden:exports:dismissed:'

function storageKey(workspaceId: number): string {
  return `${KEY_PREFIX}${workspaceId}`
}

/** Client-side only — there is no backend endpoint to delete a job record. */
export function getDismissedExportIds(workspaceId: number): Set<string> {
  try {
    const raw = localStorage.getItem(storageKey(workspaceId))
    if (!raw) return new Set()
    return new Set(JSON.parse(raw) as string[])
  } catch {
    return new Set()
  }
}

export function dismissExport(workspaceId: number, jobId: string) {
  const ids = getDismissedExportIds(workspaceId)
  ids.add(jobId)
  try {
    localStorage.setItem(storageKey(workspaceId), JSON.stringify([...ids]))
  } catch {
    // Storage unavailable/full — dismissal just won't persist across reloads.
  }
}
