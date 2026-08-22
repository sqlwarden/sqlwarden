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
}))

vi.mock('#/lib/api/queries/database', () => ({
  setConnectionTransactionMode: mocks.setConnectionTransactionMode,
  commitConnectionTransaction: mocks.commitConnectionTransaction,
  rollbackConnectionTransaction: mocks.rollbackConnectionTransaction,
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
    vi.mocked(toast.info).mockReset()
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
    store.getState().setTransactionState(7, { mode: 'manual', open: true, pendingStatements: 1 })

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

  it('commit updates the store from the response', async () => {
    store.getState().setSession(7, 'session-7')
    store.getState().setTransactionState(7, { mode: 'manual', open: true, pendingStatements: 2 })
    mocks.commitConnectionTransaction.mockResolvedValue({
      mode: 'auto',
      open: false,
      pending_statements: 0,
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
      }),
    )
    expect(mocks.commitConnectionTransaction).toHaveBeenCalledWith('acme', 3, 7, 'session-7')
  })

  it('rollback updates the store from the response', async () => {
    store.getState().setSession(7, 'session-7')
    store.getState().setTransactionState(7, { mode: 'manual', open: true, pendingStatements: 3 })
    mocks.rollbackConnectionTransaction.mockResolvedValue({
      mode: 'manual',
      open: false,
      pending_statements: 0,
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
      }),
    )
  })

  it('switchToManual calls the API and updates the store', async () => {
    store.getState().setSession(7, 'session-7')
    mocks.setConnectionTransactionMode.mockResolvedValue({
      mode: 'manual',
      open: false,
      pending_statements: 0,
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

  it('switchToManual shows a one-time informational toast when switching from auto', async () => {
    store.getState().setSession(7, 'session-7')
    store.getState().setTransactionState(7, { mode: 'auto', open: false, pendingStatements: 0 })
    mocks.setConnectionTransactionMode.mockResolvedValue({
      mode: 'manual',
      open: false,
      pending_statements: 0,
    })

    const { result } = renderHook(() => useTransactionMode('acme', 3, 7, 'session-7'), {
      wrapper,
    })

    act(() => result.current.switchToManual())

    await waitFor(() => expect(toast.info).toHaveBeenCalledTimes(1))
    expect(toast.info).toHaveBeenCalledWith(expect.stringMatching(/manual commit mode is on/i))
  })
})
