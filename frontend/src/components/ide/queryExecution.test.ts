import { describe, expect, it, vi } from 'vitest'
import type { ResultSet } from '#/lib/api/types'
import { executeQuery, type QueryExecutionDependencies } from './queryExecution'

const resultSet: ResultSet = {
  columns: [],
  rows: [],
  duration_ms: 12,
  truncated: false,
  rows_returned: 0,
  bytes_returned: 0,
}

function dependencies(
  overrides: Partial<QueryExecutionDependencies> = {},
): QueryExecutionDependencies {
  return {
    ensureSession: vi.fn(async (_connectionId, run) => run('session-1')),
    runQuery: vi.fn(async () => resultSet),
    setResult: vi.fn(),
    setRunning: vi.fn(),
    setController: vi.fn(),
    ...overrides,
  }
}

describe('executeQuery', () => {
  it('runs with an ensured session and records the successful result', async () => {
    const deps = dependencies()
    const controller = new AbortController()

    await executeQuery({ tabId: 'tab-1', connectionId: 7, sql: 'select 1', controller }, deps)

    expect(deps.ensureSession).toHaveBeenCalledWith(7, expect.any(Function), controller.signal)
    expect(deps.runQuery).toHaveBeenCalledWith('session-1', controller.signal)
    expect(deps.setResult).toHaveBeenNthCalledWith(1, 'tab-1', { status: 'running' })
    expect(deps.setResult).toHaveBeenNthCalledWith(2, 'tab-1', {
      status: 'ok',
      data: resultSet,
      durationMs: 12,
      sql: 'select 1',
      connectionId: 7,
    })
    expect(deps.setController).toHaveBeenLastCalledWith('tab-1', null)
    expect(deps.setRunning).toHaveBeenLastCalledWith('tab-1', false)
  })

  it('records cancellation and always releases transient execution state', async () => {
    const error = new Error('cancelled')
    error.name = 'AbortError'
    const deps = dependencies({
      ensureSession: vi.fn(async () => {
        throw error
      }),
    })

    await executeQuery(
      { tabId: 'tab-1', connectionId: 7, sql: 'select 1', controller: new AbortController() },
      deps,
    )

    expect(deps.setResult).toHaveBeenLastCalledWith('tab-1', {
      status: 'cancelled',
      sql: 'select 1',
    })
    expect(deps.setController).toHaveBeenLastCalledWith('tab-1', null)
    expect(deps.setRunning).toHaveBeenLastCalledWith('tab-1', false)
  })

  it('normalizes unknown failures without rejecting the UI event', async () => {
    const deps = dependencies({
      ensureSession: vi.fn(async () => {
        throw { reason: 'unknown' }
      }),
    })

    await expect(
      executeQuery(
        {
          tabId: 'tab-1',
          connectionId: 7,
          sql: 'select 1',
          controller: new AbortController(),
        },
        deps,
      ),
    ).resolves.toBeUndefined()

    expect(deps.setResult).toHaveBeenLastCalledWith('tab-1', {
      status: 'error',
      message: 'Query failed',
      sql: 'select 1',
    })
  })
})
