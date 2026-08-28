import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { TransactionState } from './useIdeStore'
import { TransactionControls } from './TransactionControls'

vi.mock('./object-detail/ReadOnlySqlView', () => ({
  ReadOnlySqlView: ({ value }: { value: string }) => (
    <pre data-testid="pending-statement-preview">{value}</pre>
  ),
}))

describe('TransactionControls', () => {
  let switchToManual: ReturnType<typeof vi.fn>
  let switchToAuto: ReturnType<typeof vi.fn>
  let commit: ReturnType<typeof vi.fn>
  let rollback: ReturnType<typeof vi.fn>
  let onSwitchToAutoBlocked: ReturnType<typeof vi.fn>

  beforeEach(() => {
    switchToManual = vi.fn()
    switchToAuto = vi.fn().mockResolvedValue('ok')
    commit = vi.fn().mockResolvedValue(undefined)
    rollback = vi.fn().mockResolvedValue(undefined)
    onSwitchToAutoBlocked = vi.fn()
  })

  function renderControls(state: TransactionState, driver = 'postgres') {
    return render(
      <TransactionControls
        state={state}
        driver={driver}
        switchToManual={switchToManual}
        switchToAuto={switchToAuto}
        commit={commit}
        rollback={rollback}
        onSwitchToAutoBlocked={onSwitchToAutoBlocked}
      />,
    )
  }

  async function openModeMenu(user: ReturnType<typeof userEvent.setup>) {
    await user.click(screen.getByRole('button', { name: /auto-commit|manual/i }))
  }

  it('shows the pending badge and Commit/Rollback buttons only when mode is manual and open', () => {
    renderControls({ mode: 'manual', open: true, pendingStatements: 4, statements: [] })

    expect(screen.getByText('4 pending')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Commit' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Rollback' })).toBeInTheDocument()
  })

  it('hides the pending badge and Commit/Rollback buttons in auto mode', () => {
    renderControls({ mode: 'auto', open: false, pendingStatements: 0, statements: [] })

    expect(screen.queryByText(/pending/)).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Commit' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Rollback' })).not.toBeInTheDocument()
  })

  it('opens a details dialog with a tab per pending statement, showing the first statement selected', async () => {
    renderControls({
      mode: 'manual',
      open: true,
      pendingStatements: 2,
      statements: ["UPDATE customer SET name = 'x'", 'SELECT 1'],
    })

    const user = userEvent.setup()
    await user.click(screen.getByText('2 pending'))

    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText('Pending statements (2)')).toBeInTheDocument()
    expect(within(dialog).getAllByRole('option')).toHaveLength(2)
    expect(within(dialog).getByRole('option', { name: /#1/ })).toHaveAttribute(
      'aria-selected',
      'true',
    )
    expect(within(dialog).getByTestId('pending-statement-preview')).toHaveTextContent(
      "UPDATE customer SET name = 'x'",
    )
  })

  it('clicking a tab shows that statement in full on the right, and wraps/scrolls long queries', async () => {
    const longStatement = `UPDATE customer SET name = 'x' WHERE ${'id = 1 OR '.repeat(50)}id = 2`
    renderControls({
      mode: 'manual',
      open: true,
      pendingStatements: 2,
      statements: ['SELECT 1', longStatement],
    })

    const user = userEvent.setup()
    await user.click(screen.getByText('2 pending'))

    const dialog = await screen.findByRole('dialog')
    await user.click(within(dialog).getByRole('option', { name: /#2/ }))

    expect(within(dialog).getByTestId('pending-statement-preview')).toHaveTextContent(longStatement)
  })

  it('disables the pending badge when there are no statements to show', () => {
    renderControls({ mode: 'manual', open: true, pendingStatements: 0, statements: [] })

    expect(screen.getByText('0 pending')).toBeDisabled()
  })

  it('shows a persistent warning in manual mode for drivers with implicit-commit DDL', () => {
    renderControls({ mode: 'manual', open: false, pendingStatements: 0, statements: [] }, 'mysql')

    expect(screen.getByTestId('manual-transaction-warning')).toBeInTheDocument()
  })

  it('hides the warning for drivers with fully transactional DDL', () => {
    renderControls(
      { mode: 'manual', open: false, pendingStatements: 0, statements: [] },
      'postgres',
    )

    expect(screen.queryByTestId('manual-transaction-warning')).not.toBeInTheDocument()
  })

  it('hides the warning in auto-commit mode even for drivers with implicit-commit DDL', () => {
    renderControls({ mode: 'auto', open: false, pendingStatements: 0, statements: [] }, 'mysql')

    expect(screen.queryByTestId('manual-transaction-warning')).not.toBeInTheDocument()
  })

  it('selecting Manual transaction calls switchToManual', async () => {
    renderControls({ mode: 'auto', open: false, pendingStatements: 0, statements: [] })

    const user = userEvent.setup()
    await openModeMenu(user)
    await user.click(await screen.findByRole('menuitem', { name: 'Manual transaction' }))

    expect(switchToManual).toHaveBeenCalledTimes(1)
  })

  it('selecting Auto-commit calls switchToAuto and does not block when it resolves ok', async () => {
    renderControls({ mode: 'manual', open: false, pendingStatements: 0, statements: [] })

    const user = userEvent.setup()
    await openModeMenu(user)
    await user.click(await screen.findByRole('menuitem', { name: 'Auto-commit' }))

    await waitFor(() => expect(switchToAuto).toHaveBeenCalledTimes(1))
    expect(onSwitchToAutoBlocked).not.toHaveBeenCalled()
  })

  it('selecting Auto-commit while open calls onSwitchToAutoBlocked when switchToAuto reports blocked', async () => {
    switchToAuto.mockResolvedValue('blocked')
    renderControls({ mode: 'manual', open: true, pendingStatements: 1, statements: [] })

    const user = userEvent.setup()
    await openModeMenu(user)
    await user.click(await screen.findByRole('menuitem', { name: 'Auto-commit' }))

    await waitFor(() => expect(onSwitchToAutoBlocked).toHaveBeenCalledTimes(1))
  })

  it('clicking Commit calls commit', async () => {
    renderControls({ mode: 'manual', open: true, pendingStatements: 1, statements: [] })

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Commit' }))

    expect(commit).toHaveBeenCalledTimes(1)
  })

  it('clicking Rollback calls rollback', async () => {
    renderControls({ mode: 'manual', open: true, pendingStatements: 1, statements: [] })

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Rollback' }))

    expect(rollback).toHaveBeenCalledTimes(1)
  })
})
