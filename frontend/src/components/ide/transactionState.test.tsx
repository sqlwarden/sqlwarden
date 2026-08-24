import type { PropsWithChildren } from 'react'
import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createIdeStore, IdeStoreContext } from './useIdeStore'
import {
  DEFAULT_TRANSACTION_STATE,
  toTransactionState,
  useRefreshTransactionState,
  useTransactionHydration,
} from './transactionState'

const mocks = vi.hoisted(() => ({
  getConnectionTransactionStatus: vi.fn(),
}))

vi.mock('#/lib/api/queries/database', () => ({
  getConnectionTransactionStatus: mocks.getConnectionTransactionStatus,
}))

describe('toTransactionState', () => {
  it('maps the wire response to the store shape', () => {
    expect(
      toTransactionState({ mode: 'manual', open: true, pending_statements: 3, statements: [] }),
    ).toEqual({
      mode: 'manual',
      open: true,
      pendingStatements: 3,
      statements: [],
    })
  })
})

describe('useRefreshTransactionState', () => {
  let store: ReturnType<typeof createIdeStore>

  beforeEach(() => {
    store = createIdeStore('acme', 1, 'ephemeral')
    mocks.getConnectionTransactionStatus.mockReset()
  })

  function wrapper({ children }: PropsWithChildren) {
    return <IdeStoreContext.Provider value={store}>{children}</IdeStoreContext.Provider>
  }

  it('resets to the auto-commit default without a request when there is no session', async () => {
    store
      .getState()
      .setTransactionState(7, { mode: 'manual', open: true, pendingStatements: 2, statements: [] })

    const { result } = renderHook(() => useRefreshTransactionState('acme', 3), { wrapper })
    await act(async () => {
      await result.current(7)
    })

    expect(mocks.getConnectionTransactionStatus).not.toHaveBeenCalled()
    expect(store.getState().transactions[7]).toEqual(DEFAULT_TRANSACTION_STATE)
  })

  it('writes the fetched status into the store on success', async () => {
    store.getState().setSession(7, 'session-7')
    mocks.getConnectionTransactionStatus.mockResolvedValue({
      mode: 'manual',
      open: true,
      pending_statements: 5,
      statements: [],
    })

    const { result } = renderHook(() => useRefreshTransactionState('acme', 3), { wrapper })
    await act(async () => {
      await result.current(7)
    })

    expect(mocks.getConnectionTransactionStatus).toHaveBeenCalledWith('acme', 3, 7, 'session-7')
    expect(store.getState().transactions[7]).toEqual({
      mode: 'manual',
      open: true,
      pendingStatements: 5,
      statements: [],
    })
  })

  it('falls back to the auto-commit default when the request fails', async () => {
    store.getState().setSession(7, 'session-7')
    store
      .getState()
      .setTransactionState(7, { mode: 'manual', open: true, pendingStatements: 2, statements: [] })
    mocks.getConnectionTransactionStatus.mockRejectedValue(new Error('network error'))

    const { result } = renderHook(() => useRefreshTransactionState('acme', 3), { wrapper })
    await act(async () => {
      await result.current(7)
    })

    expect(store.getState().transactions[7]).toEqual(DEFAULT_TRANSACTION_STATE)
  })
})

describe('useTransactionHydration', () => {
  let store: ReturnType<typeof createIdeStore>

  beforeEach(() => {
    store = createIdeStore('acme', 1, 'ephemeral')
    mocks.getConnectionTransactionStatus.mockReset()
  })

  function wrapper({ children }: PropsWithChildren) {
    return <IdeStoreContext.Provider value={store}>{children}</IdeStoreContext.Provider>
  }

  it('does nothing without a live connection and session', async () => {
    const { rerender } = renderHook(
      ({ connectionId, sessionId }: { connectionId?: number; sessionId?: string }) =>
        useTransactionHydration('acme', 3, connectionId, sessionId),
      {
        wrapper,
        initialProps: { connectionId: undefined, sessionId: undefined } as {
          connectionId?: number
          sessionId?: string
        },
      },
    )
    rerender({ connectionId: 7, sessionId: undefined })

    expect(mocks.getConnectionTransactionStatus).not.toHaveBeenCalled()
  })

  it('fetches transaction status once a connection and session are both present', async () => {
    store.getState().setSession(7, 'session-7')
    mocks.getConnectionTransactionStatus.mockResolvedValue({
      mode: 'manual',
      open: true,
      pending_statements: 1,
      statements: [],
    })

    await act(async () => {
      renderHook(() => useTransactionHydration('acme', 3, 7, 'session-7'), { wrapper })
    })

    expect(mocks.getConnectionTransactionStatus).toHaveBeenCalledWith('acme', 3, 7, 'session-7')
    expect(store.getState().transactions[7]).toEqual({
      mode: 'manual',
      open: true,
      pendingStatements: 1,
      statements: [],
    })
  })

  it('re-fetches when the session id changes, e.g. on reconnect', async () => {
    store.getState().setSession(7, 'session-1')
    mocks.getConnectionTransactionStatus.mockResolvedValue({
      mode: 'auto',
      open: false,
      pending_statements: 0,
      statements: [],
    })

    const { rerender } = renderHook(
      ({ sessionId }: { sessionId: string }) => useTransactionHydration('acme', 3, 7, sessionId),
      { wrapper, initialProps: { sessionId: 'session-1' } },
    )
    await act(async () => {})
    expect(mocks.getConnectionTransactionStatus).toHaveBeenCalledTimes(1)

    await act(async () => {
      store.getState().setSession(7, 'session-2')
      rerender({ sessionId: 'session-2' })
    })
    expect(mocks.getConnectionTransactionStatus).toHaveBeenCalledTimes(2)
    expect(mocks.getConnectionTransactionStatus).toHaveBeenLastCalledWith('acme', 3, 7, 'session-2')
  })
})
