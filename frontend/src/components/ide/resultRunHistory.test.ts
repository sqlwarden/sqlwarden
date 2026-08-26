import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ResultRun } from './useIdeStore'

const mocks = vi.hoisted(() => ({ closeCursor: vi.fn() }))
vi.mock('#/lib/api/query', () => ({ closeConnectionQueryCursor: mocks.closeCursor }))

import { closeRunCursors, RUN_HISTORY_CAP, runsToEvict } from './resultRunHistory'

function run(id: string, results: ResultRun['results'] = []): ResultRun {
  return { id, tabId: 'tab-1', results, selectedIndex: 0, createdAt: 0 }
}

describe('runsToEvict', () => {
  it('returns nothing when the history is under the cap', () => {
    const runs = Array.from({ length: RUN_HISTORY_CAP }, (_, i) => run(`run-${i}`))
    expect(runsToEvict(runs)).toEqual([])
  })

  it('returns the oldest runs when the history exceeds the cap', () => {
    const runs = Array.from({ length: RUN_HISTORY_CAP + 2 }, (_, i) => run(`run-${i}`))
    const evicted = runsToEvict(runs)
    expect(evicted.map((r) => r.id)).toEqual(['run-0', 'run-1'])
  })

  it('never evicts pinned runs, and does not count them against the cap', () => {
    const runs = [
      { ...run('pinned-0'), pinned: true },
      ...Array.from({ length: RUN_HISTORY_CAP + 1 }, (_, i) => run(`run-${i}`)),
    ]
    const evicted = runsToEvict(runs)
    expect(evicted.map((r) => r.id)).toEqual(['run-0'])
  })
})

describe('closeRunCursors', () => {
  beforeEach(() => {
    mocks.closeCursor.mockReset().mockResolvedValue(undefined)
  })

  it('closes the cursor for every ok result carrying one', async () => {
    const evicted = run('run-1', [
      {
        status: 'ok',
        sql: 'select 1',
        durationMs: 1,
        connectionId: 9,
        data: {
          columns: [],
          rows: [],
          duration_ms: 1,
          truncated: false,
          rows_returned: 0,
          bytes_returned: 0,
          transaction: { mode: 'auto', open: false, pending_statements: 0, statements: [] },
          query_cursor_id: 'cursor-1',
        },
      },
      { status: 'error', sql: 'select 2', message: 'boom' },
    ])

    await closeRunCursors('acme', 3, 7, evicted)

    expect(mocks.closeCursor).toHaveBeenCalledTimes(1)
    expect(mocks.closeCursor).toHaveBeenCalledWith('acme', 3, 9, 'cursor-1')
  })

  it('skips ok results with no cursor', async () => {
    const evicted = run('run-1', [
      {
        status: 'ok',
        sql: 'select 1',
        durationMs: 1,
        connectionId: 9,
        data: {
          columns: [],
          rows: [],
          duration_ms: 1,
          truncated: false,
          rows_returned: 0,
          bytes_returned: 0,
          transaction: { mode: 'auto', open: false, pending_statements: 0, statements: [] },
        },
      },
    ])

    await closeRunCursors('acme', 3, 7, evicted)

    expect(mocks.closeCursor).not.toHaveBeenCalled()
  })

  it('falls back to the given connection id when a result has none', async () => {
    const evicted = run('run-1', [
      {
        status: 'ok',
        sql: 'select 1',
        durationMs: 1,
        data: {
          columns: [],
          rows: [],
          duration_ms: 1,
          truncated: false,
          rows_returned: 0,
          bytes_returned: 0,
          transaction: { mode: 'auto', open: false, pending_statements: 0, statements: [] },
          query_cursor_id: 'cursor-1',
        },
      },
    ])

    await closeRunCursors('acme', 3, 7, evicted)

    expect(mocks.closeCursor).toHaveBeenCalledWith('acme', 3, 7, 'cursor-1')
  })
})
