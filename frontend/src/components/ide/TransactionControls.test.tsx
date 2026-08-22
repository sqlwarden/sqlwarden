import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { TransactionState } from './useIdeStore'
import { TransactionControls } from './TransactionControls'

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

  function renderControls(state: TransactionState) {
    return render(
      <TransactionControls
        state={state}
        switchToManual={switchToManual}
        switchToAuto={switchToAuto}
        commit={commit}
        rollback={rollback}
        onSwitchToAutoBlocked={onSwitchToAutoBlocked}
      />,
    )
  }

  it('shows the pending badge and Commit/Rollback buttons only when mode is manual and open', () => {
    renderControls({ mode: 'manual', open: true, pendingStatements: 4 })

    expect(screen.getByText('4 pending')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Commit' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Rollback' })).toBeInTheDocument()
  })

  it('hides the pending badge and Commit/Rollback buttons in auto mode', () => {
    renderControls({ mode: 'auto', open: false, pendingStatements: 0 })

    expect(screen.queryByText(/pending/)).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Commit' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Rollback' })).not.toBeInTheDocument()
  })

  it('toggling on calls switchToManual', async () => {
    renderControls({ mode: 'auto', open: false, pendingStatements: 0 })

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Manual transaction mode' }))

    expect(switchToManual).toHaveBeenCalledTimes(1)
  })

  it('toggling off calls switchToAuto and does not block when it resolves ok', async () => {
    renderControls({ mode: 'manual', open: false, pendingStatements: 0 })

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Manual transaction mode' }))

    await waitFor(() => expect(switchToAuto).toHaveBeenCalledTimes(1))
    expect(onSwitchToAutoBlocked).not.toHaveBeenCalled()
  })

  it('toggling off while open calls onSwitchToAutoBlocked when switchToAuto reports blocked', async () => {
    switchToAuto.mockResolvedValue('blocked')
    renderControls({ mode: 'manual', open: true, pendingStatements: 1 })

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Manual transaction mode' }))

    await waitFor(() => expect(onSwitchToAutoBlocked).toHaveBeenCalledTimes(1))
  })

  it('clicking Commit calls commit', async () => {
    renderControls({ mode: 'manual', open: true, pendingStatements: 1 })

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Commit' }))

    expect(commit).toHaveBeenCalledTimes(1)
  })

  it('clicking Rollback calls rollback', async () => {
    renderControls({ mode: 'manual', open: true, pendingStatements: 1 })

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Rollback' }))

    expect(rollback).toHaveBeenCalledTimes(1)
  })
})
