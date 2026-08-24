import type { PropsWithChildren } from 'react'
import { QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'sonner'
import { createTestQueryClient } from '#/test/render'
import { createIdeStore, IdeStoreContext } from './useIdeStore'
import { useTransactionMode } from './useTransactionMode'

const mocks = vi.hoisted(() => ({
  setConnectionTransactionMode: vi.fn(),
  commitConnectionTransaction: vi.fn(),
  rollbackConnectionTransaction: vi.fn(),
  getConnectionTransactionStatus: vi.fn(),
}))

vi.mock('#/lib/api/queries/database', () => ({
  setConnectionTransactionMode: mocks.setConnectionTransactionMode,
  commitConnectionTransaction: mocks.commitConnectionTransaction,
  rollbackConnectionTransaction: mocks.rollbackConnectionTransaction,
  getConnectionTransactionStatus: mocks.getConnectionTransactionStatus,
}))

vi.mock('sonner', () => ({ toast: { error: vi.fn(), info: vi.fn() } }))

describe('useTransactionMode', () => {
  let store: ReturnType<typeof createIdeStore>
  const queryClient = createTestQueryClient()

  beforeEach(() => {
    store = createIdeStore('acme', 1, 'ephemeral')
    queryClient.clear()
    mocks.setConnectionTransactionMode.mockReset()
    mocks.commitConnectionTransaction.mockReset()
    mocks.rollbackConnectionTransaction.mockReset()
    mocks.getConnectionTransactionStatus.mockReset()
    vi.mocked(toast.info).mockReset()
    vi.mocked(toast.error).mockReset()
  })

  function wrapper({ children }: PropsWithChildren) {
    return (
      <QueryClientProvider client={queryClient}>
        <IdeStoreContext.Provider value={store}>{children}</IdeStoreContext.Provider>
      </QueryClientProvider>
    )
  }

  it('switchToAuto returns blocked without calling the API when a transaction is open', async () => {
    store.getState().setSession(7, 'session-7')
    store
      .getState()
      .setTransactionState(7, { mode: 'manual', open: true, pendingStatements: 1, statements: [] })

    const { result } = renderHook(() => useTransactionMode('acme', 3, 7, 'session-7'), {
      wrapper,
    })

    let outcome: 'ok' | 'blocked' | undefined
    await act(async () => {
      outcome = await result.current.switchToAuto()
    })

    expect(outcome).toBe('blocked')
    expect(mocks.setConnectionTransactionMode).not.toHaveBeenCalled()
  })

  it("switchToAuto reads live state instead of the closure it was captured in, so commit-then-switchToAuto (the guard dialog's pattern) actually switches", async () => {
    store.getState().setSession(7, 'session-7')
    store
      .getState()
      .setTransactionState(7, { mode: 'manual', open: true, pendingStatements: 2, statements: [] })
    mocks.commitConnectionTransaction.mockResolvedValue({
      mode: 'manual',
      open: false,
      pending_statements: 0,
      statements: [],
    })
    mocks.setConnectionTransactionMode.mockResolvedValue({
      mode: 'auto',
      open: false,
      pending_statements: 0,
      statements: [],
    })

    const { result } = renderHook(() => useTransactionMode('acme', 3, 7, 'session-7'), {
      wrapper,
    })
    // Captures switchToAuto from this render, where state.open is still true —
    // the guard dialog's onCommit handler does the same: it closes over the
    // hook's return value from the render that opened the dialog.
    const { commit, switchToAuto } = result.current

    let outcome: 'ok' | 'blocked' | undefined
    await act(async () => {
      await commit()
      outcome = await switchToAuto()
    })

    expect(outcome).toBe('ok')
    expect(mocks.setConnectionTransactionMode).toHaveBeenCalledWith(
      'acme',
      3,
      7,
      'session-7',
      'auto',
    )
  })

  it('commit updates the store from the response', async () => {
    store.getState().setSession(7, 'session-7')
    store
      .getState()
      .setTransactionState(7, { mode: 'manual', open: true, pendingStatements: 2, statements: [] })
    mocks.commitConnectionTransaction.mockResolvedValue({
      mode: 'auto',
      open: false,
      pending_statements: 0,
      statements: [],
    })

    const { result } = renderHook(() => useTransactionMode('acme', 3, 7, 'session-7'), {
      wrapper,
    })

    await act(async () => {
      await result.current.commit()
    })

    await waitFor(() =>
      expect(store.getState().transactions[7]).toEqual({
        mode: 'auto',
        open: false,
        pendingStatements: 0,
        statements: [],
      }),
    )
    expect(mocks.commitConnectionTransaction).toHaveBeenCalledWith('acme', 3, 7, 'session-7')
  })

  it('rollback updates the store from the response', async () => {
    store.getState().setSession(7, 'session-7')
    store
      .getState()
      .setTransactionState(7, { mode: 'manual', open: true, pendingStatements: 3, statements: [] })
    mocks.rollbackConnectionTransaction.mockResolvedValue({
      mode: 'manual',
      open: false,
      pending_statements: 0,
      statements: [],
    })

    const { result } = renderHook(() => useTransactionMode('acme', 3, 7, 'session-7'), {
      wrapper,
    })

    await act(async () => {
      await result.current.rollback()
    })

    await waitFor(() =>
      expect(store.getState().transactions[7]).toEqual({
        mode: 'manual',
        open: false,
        pendingStatements: 0,
        statements: [],
      }),
    )
  })

  it('commit resyncs from the backend when the connection died, instead of leaving the stale open transaction on screen', async () => {
    store.getState().setSession(7, 'session-7')
    store
      .getState()
      .setTransactionState(7, { mode: 'manual', open: true, pendingStatements: 2, statements: [] })
    mocks.commitConnectionTransaction.mockRejectedValue(new Error('connection reset by peer'))
    // The backend already discarded the transaction once the driver call
    // failed (see internal/connection CommitTransaction) — refetching status
    // reflects that, not the stale state the failed mutation would otherwise
    // leave behind.
    mocks.getConnectionTransactionStatus.mockResolvedValue({
      mode: 'manual',
      open: false,
      pending_statements: 0,
      statements: [],
    })

    const { result } = renderHook(() => useTransactionMode('acme', 3, 7, 'session-7'), {
      wrapper,
    })

    await act(async () => {
      await expect(result.current.commit()).rejects.toThrow()
    })

    await waitFor(() =>
      expect(store.getState().transactions[7]).toEqual({
        mode: 'manual',
        open: false,
        pendingStatements: 0,
        statements: [],
      }),
    )
    expect(toast.error).toHaveBeenCalled()
  })

  it('rollback resyncs from the backend when the connection died', async () => {
    store.getState().setSession(7, 'session-7')
    store
      .getState()
      .setTransactionState(7, { mode: 'manual', open: true, pendingStatements: 1, statements: [] })
    mocks.rollbackConnectionTransaction.mockRejectedValue(new Error('connection reset by peer'))
    mocks.getConnectionTransactionStatus.mockResolvedValue({
      mode: 'manual',
      open: false,
      pending_statements: 0,
      statements: [],
    })

    const { result } = renderHook(() => useTransactionMode('acme', 3, 7, 'session-7'), {
      wrapper,
    })

    await act(async () => {
      await expect(result.current.rollback()).rejects.toThrow()
    })

    await waitFor(() =>
      expect(store.getState().transactions[7]).toEqual({
        mode: 'manual',
        open: false,
        pendingStatements: 0,
        statements: [],
      }),
    )
  })

  it('commit resyncs to the auto-commit default when the session itself is gone', async () => {
    store.getState().setSession(7, 'session-7')
    store
      .getState()
      .setTransactionState(7, { mode: 'manual', open: true, pendingStatements: 1, statements: [] })
    mocks.commitConnectionTransaction.mockRejectedValue(new Error('session not found'))
    mocks.getConnectionTransactionStatus.mockRejectedValue(new Error('session not found'))

    const { result } = renderHook(() => useTransactionMode('acme', 3, 7, 'session-7'), {
      wrapper,
    })

    await act(async () => {
      await expect(result.current.commit()).rejects.toThrow()
    })

    await waitFor(() =>
      expect(store.getState().transactions[7]).toEqual({
        mode: 'auto',
        open: false,
        pendingStatements: 0,
        statements: [],
      }),
    )
  })

  it('switchToManual resyncs from the backend on failure instead of leaving stale state', async () => {
    store.getState().setSession(7, 'session-7')
    mocks.setConnectionTransactionMode.mockRejectedValue(new Error('connection reset by peer'))
    mocks.getConnectionTransactionStatus.mockResolvedValue({
      mode: 'auto',
      open: false,
      pending_statements: 0,
      statements: [],
    })

    const { result } = renderHook(() => useTransactionMode('acme', 3, 7, 'session-7'), {
      wrapper,
    })

    act(() => result.current.switchToManual())

    await waitFor(() => expect(mocks.getConnectionTransactionStatus).toHaveBeenCalled())
    await waitFor(() =>
      expect(store.getState().transactions[7]).toEqual({
        mode: 'auto',
        open: false,
        pendingStatements: 0,
        statements: [],
      }),
    )
  })

  it('switchToManual calls the API and updates the store', async () => {
    store.getState().setSession(7, 'session-7')
    mocks.setConnectionTransactionMode.mockResolvedValue({
      mode: 'manual',
      open: false,
      pending_statements: 0,
      statements: [],
    })

    const { result } = renderHook(() => useTransactionMode('acme', 3, 7, 'session-7'), {
      wrapper,
    })

    act(() => result.current.switchToManual())

    await waitFor(() =>
      expect(store.getState().transactions[7]).toEqual({
        mode: 'manual',
        open: false,
        pendingStatements: 0,
        statements: [],
      }),
    )
    expect(mocks.setConnectionTransactionMode).toHaveBeenCalledWith(
      'acme',
      3,
      7,
      'session-7',
      'manual',
    )
  })
})
