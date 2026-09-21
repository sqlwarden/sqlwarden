import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { createIdeStore, IdeStoreContext } from './useIdeStore'
import { TransactionStatus } from './TransactionStatus'

vi.mock('./object-detail/ReadOnlySqlView', () => ({
  ReadOnlySqlView: ({ value }: { value: string }) => <pre>{value}</pre>,
}))

function renderStatus(
  setup: (store: ReturnType<typeof createIdeStore>) => void,
  connectionId: number | undefined = 7,
) {
  const store = createIdeStore('acme', 1, 'ephemeral')
  setup(store)
  return render(
    <IdeStoreContext.Provider value={store}>
      <TransactionStatus connectionId={connectionId} />
    </IdeStoreContext.Provider>,
  )
}

describe('TransactionStatus', () => {
  it('renders nothing without a live session', () => {
    renderStatus((store) =>
      store.getState().setTransactionState(7, {
        mode: 'manual',
        open: true,
        pendingStatements: 2,
        statements: ['SELECT 1', 'SELECT 2'],
      }),
    )
    expect(screen.queryByText(/Transaction open/)).not.toBeInTheDocument()
  })

  it('renders nothing while no manual transaction is open', () => {
    renderStatus((store) => {
      store.getState().setSession(7, 'sess')
      store.getState().setTransactionState(7, {
        mode: 'manual',
        open: false,
        pendingStatements: 0,
        statements: [],
      })
    })
    expect(screen.queryByText(/Transaction open/)).not.toBeInTheDocument()
  })

  it('shows the statement count and opens the pending statements dialog', async () => {
    renderStatus((store) => {
      store.getState().setSession(7, 'sess')
      store.getState().setTransactionState(7, {
        mode: 'manual',
        open: true,
        pendingStatements: 2,
        statements: ['SELECT 1', 'SELECT 2'],
      })
    })

    const item = screen.getByRole('button', { name: /Transaction open · 2 statements/ })
    await userEvent.setup().click(item)

    expect(await screen.findByRole('dialog')).toHaveTextContent('Pending statements (2)')
  })
})
