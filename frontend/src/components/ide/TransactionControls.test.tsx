import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createTestQueryClient } from '#/test/render'
import { createIdeStore, IdeStoreContext } from './useIdeStore'
import { TransactionControls } from './TransactionControls'

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

vi.mock('sonner', () => ({ toast: { error: vi.fn() } }))

describe('TransactionControls', () => {
  let store: ReturnType<typeof createIdeStore>
  let queryClient: ReturnType<typeof createTestQueryClient>
  let onSwitchToAutoBlocked: ReturnType<typeof vi.fn>

  beforeEach(() => {
    store = createIdeStore('acme', 1, 'ephemeral')
    queryClient = createTestQueryClient()
    onSwitchToAutoBlocked = vi.fn()
    mocks.setConnectionTransactionMode.mockReset()
    mocks.commitConnectionTransaction.mockReset()
    mocks.rollbackConnectionTransaction.mockReset()
  })

  function renderControls(connectionId: number | undefined, sessionId: string | undefined) {
    return render(
      <QueryClientProvider client={queryClient}>
        <IdeStoreContext.Provider value={store}>
          <TransactionControls
            orgSlug="acme"
            workspaceId={3}
            connectionId={connectionId}
            sessionId={sessionId}
            onSwitchToAutoBlocked={onSwitchToAutoBlocked}
          />
        </IdeStoreContext.Provider>
      </QueryClientProvider>,
    )
  }

  it('renders nothing when connectionId is undefined', () => {
    const { container } = renderControls(undefined, 'session-7')
    expect(container).toBeEmptyDOMElement()
  })

  it('renders nothing when sessionId is undefined', () => {
    const { container } = renderControls(7, undefined)
    expect(container).toBeEmptyDOMElement()
  })

  it('shows the pending badge and Commit/Rollback buttons only when mode is manual and open', () => {
    store.getState().setTransactionState(7, { mode: 'manual', open: true, pendingStatements: 4 })
    renderControls(7, 'session-7')

    expect(screen.getByText('4 pending')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Commit' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Rollback' })).toBeInTheDocument()
  })

  it('hides the pending badge and Commit/Rollback buttons in auto mode', () => {
    store.getState().setTransactionState(7, { mode: 'auto', open: false, pendingStatements: 0 })
    renderControls(7, 'session-7')

    expect(screen.queryByText(/pending/)).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Commit' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Rollback' })).not.toBeInTheDocument()
  })

  it('toggling on calls switchToManual', async () => {
    mocks.setConnectionTransactionMode.mockResolvedValue({
      mode: 'manual',
      open: false,
      pending_statements: 0,
    })
    store.getState().setTransactionState(7, { mode: 'auto', open: false, pendingStatements: 0 })
    renderControls(7, 'session-7')

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Manual transaction mode' }))

    await waitFor(() =>
      expect(mocks.setConnectionTransactionMode).toHaveBeenCalledWith(
        'acme',
        3,
        7,
        'session-7',
        'manual',
      ),
    )
  })

  it('toggling off while open calls onSwitchToAutoBlocked instead of the API', async () => {
    store.getState().setTransactionState(7, { mode: 'manual', open: true, pendingStatements: 1 })
    renderControls(7, 'session-7')

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Manual transaction mode' }))

    await waitFor(() => expect(onSwitchToAutoBlocked).toHaveBeenCalledTimes(1))
    expect(mocks.setConnectionTransactionMode).not.toHaveBeenCalled()
  })
})
