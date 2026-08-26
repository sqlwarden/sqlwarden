// Pure, React-free helpers for scoping the results panel's visible runs by
// resultsPanelMode. Runs always live in `resultRuns` keyed by their origin
// tab (see useIdeStore.ts); this module only decides which subset to show
// and which one counts as "selected" — it never mutates run storage itself.
import type { ResultRun, ResultsPanelMode } from './useIdeStore'

/** Runs visible in the results panel for the given mode.
 *  - 'per-editor': only the focused tab's own runs, in their stored order.
 *  - 'per-connection': every run (from any tab) that ran against the
 *    focused tab's connection, oldest first.
 *  - 'shared': every run from every tab, oldest first. */
export function visibleRuns(
  mode: ResultsPanelMode,
  resultRuns: Record<string, ResultRun[]>,
  activeTabId: string | undefined,
  activeConnectionId: number | undefined,
): ResultRun[] {
  if (mode === 'per-editor') {
    return activeTabId ? (resultRuns[activeTabId] ?? []) : []
  }

  const all = Object.values(resultRuns).flat()
  if (mode === 'shared') {
    return all.sort((a, b) => a.createdAt - b.createdAt)
  }

  // per-connection
  if (activeConnectionId === undefined) return []
  return all
    .filter((run) => run.connectionId === activeConnectionId)
    .sort((a, b) => a.createdAt - b.createdAt)
}

/** Which run id counts as "selected" for the given mode, falling back to the
 *  most recent visible run when the remembered selection isn't in view
 *  (evicted, closed, or never set). */
export function resolveSelectedRunId(
  mode: ResultsPanelMode,
  runs: ResultRun[],
  perTabSelectedRunId: string | undefined,
  sharedSelectedRunId: string | undefined,
  connectionSelectedRunId: string | undefined,
): string | undefined {
  const remembered =
    mode === 'per-editor'
      ? perTabSelectedRunId
      : mode === 'shared'
        ? sharedSelectedRunId
        : connectionSelectedRunId
  if (remembered && runs.some((r) => r.id === remembered)) return remembered
  return runs[runs.length - 1]?.id
}
