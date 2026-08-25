import { describe, expect, it, vi } from 'vitest'
import { ApiError } from '#/lib/api/errors'
import type { ResultSet } from '#/lib/api/types'
import { runStatementBatch, type BatchExecutionDependencies } from './runStatementBatch'

function resultSet(durationMs: number): ResultSet {
  return {
    columns: [],
    rows: [],
    duration_ms: durationMs,
    truncated: false,
    rows_returned: 0,
    bytes_returned: 0,
    transaction: { mode: 'auto', open: false, pending_statements: 0, statements: [] },
  }
}

function dependencies(
  overrides: Partial<BatchExecutionDependencies> = {},
): BatchExecutionDependencies {
  return {
    ensureSession: vi.fn(async (_connectionId, run) => run('session-1')),
    runStatement: vi.fn(async () => resultSet(1)),
    beginRun: vi.fn(() => 'run-1'),
    setRunStatementResult: vi.fn(),
    markRunRemainingSkipped: vi.fn(),
    setRunning: vi.fn(),
    setController: vi.fn(),
    setPendingConfirmation: vi.fn(),
    recordHistory: vi.fn(),
    setTransactionState: vi.fn(),
    refreshTransactionState: vi.fn(async () => undefined),
    ...overrides,
  }
}

describe('runStatementBatch', () => {
  it('runs every statement in order over the same session and finishes clean', async () => {
    const runStatement = vi.fn(async (_sessionId: string, sql: string) => resultSet(sql.length))
    const deps = dependencies({ runStatement })
    const controller = new AbortController()

    const runId = await runStatementBatch(
      { tabId: 'tab-1', connectionId: 7, sqls: ['select 1', 'select 2'], controller },
      deps,
    )

    expect(runId).toBe('run-1')
    expect(deps.beginRun).toHaveBeenCalledWith('tab-1', ['select 1', 'select 2'])
    expect(deps.ensureSession).toHaveBeenCalledTimes(2)
    expect(runStatement).toHaveBeenNthCalledWith(
      1,
      'session-1',
      'select 1',
      false,
      controller.signal,
    )
    expect(runStatement).toHaveBeenNthCalledWith(
      2,
      'session-1',
      'select 2',
      false,
      controller.signal,
    )
    expect(deps.setRunStatementResult).toHaveBeenNthCalledWith(1, 'tab-1', 'run-1', 0, {
      status: 'running',
      sql: 'select 1',
    })
    expect(deps.setRunStatementResult).toHaveBeenNthCalledWith(2, 'tab-1', 'run-1', 0, {
      status: 'ok',
      data: resultSet(8),
      durationMs: 8,
      sql: 'select 1',
      connectionId: 7,
    })
    expect(deps.setRunStatementResult).toHaveBeenNthCalledWith(3, 'tab-1', 'run-1', 1, {
      status: 'running',
      sql: 'select 2',
    })
    expect(deps.setRunStatementResult).toHaveBeenNthCalledWith(4, 'tab-1', 'run-1', 1, {
      status: 'ok',
      data: resultSet(8),
      durationMs: 8,
      sql: 'select 2',
      connectionId: 7,
    })
    expect(deps.markRunRemainingSkipped).not.toHaveBeenCalled()
    expect(deps.setController).toHaveBeenLastCalledWith('tab-1', null)
    expect(deps.setRunning).toHaveBeenLastCalledWith('tab-1', false)
    expect(deps.recordHistory).toHaveBeenNthCalledWith(1, {
      sqlText: 'select 1',
      status: 'ok',
      durationMs: 8,
      rowsAffected: 0,
    })
    expect(deps.recordHistory).toHaveBeenNthCalledWith(2, {
      sqlText: 'select 2',
      status: 'ok',
      durationMs: 8,
      rowsAffected: 0,
    })
  })

  it('syncs transaction state from the response on every successful statement', async () => {
    const runStatement = vi.fn(async () => ({
      ...resultSet(1),
      transaction: { mode: 'manual' as const, open: true, pending_statements: 1, statements: [] },
    }))
    const deps = dependencies({ runStatement })
    const controller = new AbortController()

    await runStatementBatch(
      { tabId: 'tab-1', connectionId: 7, sqls: ['select 1'], controller },
      deps,
    )

    expect(deps.setTransactionState).toHaveBeenCalledWith(7, {
      mode: 'manual',
      open: true,
      pendingStatements: 1,
      statements: [],
    })
  })

  it('stops at the first error and marks every later statement skipped', async () => {
    const runStatement = vi.fn(async (_sessionId: string, sql: string) => {
      if (sql === 'select 2') throw new Error('boom')
      return resultSet(1)
    })
    const deps = dependencies({ runStatement })
    const controller = new AbortController()

    await runStatementBatch(
      {
        tabId: 'tab-1',
        connectionId: 7,
        sqls: ['select 1', 'select 2', 'select 3'],
        controller,
      },
      deps,
    )

    expect(runStatement).toHaveBeenCalledTimes(2)
    expect(deps.setRunStatementResult).toHaveBeenCalledWith('tab-1', 'run-1', 1, {
      status: 'error',
      message: 'boom',
      sql: 'select 2',
    })
    expect(deps.markRunRemainingSkipped).toHaveBeenCalledWith('tab-1', 'run-1', 2)
    expect(deps.setRunning).toHaveBeenLastCalledWith('tab-1', false)
    expect(deps.recordHistory).toHaveBeenCalledWith({
      sqlText: 'select 2',
      status: 'error',
      errorMessage: 'boom',
      durationMs: 0,
      rowsAffected: 0,
    })
    expect(deps.refreshTransactionState).toHaveBeenCalledWith(7)
  })

  it('marks the in-flight statement cancelled and the rest skipped on abort', async () => {
    const abortError = new Error('cancelled')
    abortError.name = 'AbortError'
    const runStatement = vi.fn(async () => {
      throw abortError
    })
    const deps = dependencies({ runStatement })
    const controller = new AbortController()

    await runStatementBatch(
      { tabId: 'tab-1', connectionId: 7, sqls: ['select 1', 'select 2'], controller },
      deps,
    )

    expect(deps.setRunStatementResult).toHaveBeenCalledWith('tab-1', 'run-1', 0, {
      status: 'cancelled',
      sql: 'select 1',
    })
    expect(deps.markRunRemainingSkipped).toHaveBeenCalledWith('tab-1', 'run-1', 1)
    expect(deps.recordHistory).toHaveBeenCalledWith({
      sqlText: 'select 1',
      status: 'cancelled',
      durationMs: 0,
      rowsAffected: 0,
    })
    expect(deps.refreshTransactionState).toHaveBeenCalledWith(7)
  })

  it('pauses on an unsafe-confirmation error without marking anything skipped', async () => {
    const error = new ApiError('Confirm to run it anyway.', 422, {
      code: 'unsafe_query_confirmation_required',
      details: [{ kind: 'unsafe_missing_where', start_offset: 0, end_offset: 18 }],
    })
    const runStatement = vi.fn(async () => {
      throw error
    })
    const deps = dependencies({ runStatement })
    const controller = new AbortController()

    await runStatementBatch(
      {
        tabId: 'tab-1',
        connectionId: 7,
        sqls: ['DELETE FROM widgets', 'select 2'],
        controller,
      },
      deps,
    )

    expect(deps.setRunStatementResult).toHaveBeenCalledWith('tab-1', 'run-1', 0, {
      status: 'pending',
      sql: 'DELETE FROM widgets',
    })
    expect(deps.setPendingConfirmation).toHaveBeenCalledWith('tab-1', {
      sql: 'DELETE FROM widgets',
      statements: [{ kind: 'unsafe_missing_where', start_offset: 0, end_offset: 18 }],
      runId: 'run-1',
      statementIndex: 0,
    })
    expect(deps.markRunRemainingSkipped).not.toHaveBeenCalled()
    expect(deps.setController).toHaveBeenLastCalledWith('tab-1', null)
    expect(deps.setRunning).toHaveBeenLastCalledWith('tab-1', false)
  })

  it('resumes from startAt with confirmUnsafe only at confirmUnsafeAt, reusing the given runId', async () => {
    const confirmFlags: boolean[] = []
    const runStatement = vi.fn(async (_sessionId: string, _sql: string, confirmUnsafe: boolean) => {
      confirmFlags.push(confirmUnsafe)
      return resultSet(1)
    })
    const deps = dependencies({ runStatement })
    const controller = new AbortController()

    const runId = await runStatementBatch(
      {
        tabId: 'tab-1',
        connectionId: 7,
        sqls: ['DELETE FROM widgets', 'select 2'],
        controller,
        runId: 'existing-run',
        startAt: 0,
        confirmUnsafeAt: 0,
      },
      deps,
    )

    expect(runId).toBe('existing-run')
    expect(deps.beginRun).not.toHaveBeenCalled()
    expect(confirmFlags).toEqual([true, false])
  })

  it('confirming an unsafe DML inside a manual transaction syncs the resulting pending count into the store', async () => {
    const runStatement = vi.fn(
      async (_sessionId: string, _sql: string, confirmUnsafe: boolean) => ({
        ...resultSet(1),
        transaction: {
          mode: 'manual' as const,
          open: true,
          pending_statements: confirmUnsafe ? 1 : 0,
          statements: confirmUnsafe ? ['DELETE FROM widgets'] : [],
        },
      }),
    )
    const deps = dependencies({ runStatement })
    const controller = new AbortController()

    await runStatementBatch(
      {
        tabId: 'tab-1',
        connectionId: 7,
        sqls: ['DELETE FROM widgets'],
        controller,
        runId: 'existing-run',
        startAt: 0,
        confirmUnsafeAt: 0,
      },
      deps,
    )

    expect(deps.setTransactionState).toHaveBeenCalledWith(7, {
      mode: 'manual',
      open: true,
      pendingStatements: 1,
      statements: ['DELETE FROM widgets'],
    })
  })

  it('resyncs transaction state when a confirmed unsafe statement still fails (e.g. the connection died)', async () => {
    const runStatement = vi.fn(async () => {
      throw new Error('connection reset by peer')
    })
    const deps = dependencies({ runStatement })
    const controller = new AbortController()

    await runStatementBatch(
      {
        tabId: 'tab-1',
        connectionId: 7,
        sqls: ['DELETE FROM widgets'],
        controller,
        runId: 'existing-run',
        startAt: 0,
        confirmUnsafeAt: 0,
      },
      deps,
    )

    expect(deps.setRunStatementResult).toHaveBeenCalledWith('tab-1', 'existing-run', 0, {
      status: 'error',
      message: 'connection reset by peer',
      sql: 'DELETE FROM widgets',
    })
    expect(deps.refreshTransactionState).toHaveBeenCalledWith(7)
  })

  it('runs a single statement (the Run button path) as a one-statement batch', async () => {
    const deps = dependencies()
    const controller = new AbortController()

    const runId = await runStatementBatch(
      { tabId: 'tab-1', connectionId: 7, sqls: ['select 1'], controller },
      deps,
    )

    expect(runId).toBe('run-1')
    expect(deps.beginRun).toHaveBeenCalledWith('tab-1', ['select 1'])
    expect(deps.setRunStatementResult).toHaveBeenNthCalledWith(2, 'tab-1', 'run-1', 0, {
      status: 'ok',
      data: resultSet(1),
      durationMs: 1,
      sql: 'select 1',
      connectionId: 7,
    })
    expect(deps.recordHistory).toHaveBeenCalledWith({
      sqlText: 'select 1',
      status: 'ok',
      durationMs: 1,
      rowsAffected: 0,
    })
  })

  it('normalizes an unknown single-statement failure without rejecting the UI event', async () => {
    const deps = dependencies({
      ensureSession: vi.fn(async () => {
        throw { reason: 'unknown' }
      }),
    })
    const controller = new AbortController()

    await expect(
      runStatementBatch({ tabId: 'tab-1', connectionId: 7, sqls: ['select 1'], controller }, deps),
    ).resolves.toBe('run-1')

    expect(deps.setRunStatementResult).toHaveBeenLastCalledWith('tab-1', 'run-1', 0, {
      status: 'error',
      message: 'Query failed',
      sql: 'select 1',
    })
    expect(deps.recordHistory).toHaveBeenCalledWith({
      sqlText: 'select 1',
      status: 'error',
      errorMessage: 'Query failed',
      durationMs: 0,
      rowsAffected: 0,
    })
  })
})
