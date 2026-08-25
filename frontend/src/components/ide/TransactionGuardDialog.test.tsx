import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { TransactionGuardDialog } from './TransactionGuardDialog'

describe('TransactionGuardDialog', () => {
  it('renders switch-to-auto copy with the pending statement count', () => {
    render(
      <TransactionGuardDialog
        open
        reason="switch-to-auto"
        pendingStatements={1}
        onCommit={vi.fn()}
        onRollback={vi.fn()}
        onOpenChange={vi.fn()}
      />,
    )
    const dialog = screen.getByRole('alertdialog')
    expect(dialog).toHaveTextContent(/switching to auto-commit/i)
    expect(dialog).toHaveTextContent('1 statement pending.')
  })

  it('renders close-connection copy with the pending statement count', () => {
    render(
      <TransactionGuardDialog
        open
        reason="close-connection"
        pendingStatements={3}
        onCommit={vi.fn()}
        onRollback={vi.fn()}
        onOpenChange={vi.fn()}
      />,
    )
    const dialog = screen.getByRole('alertdialog')
    expect(dialog).toHaveTextContent(/closing without resolving it/i)
    expect(dialog).toHaveTextContent('3 statements pending.')
  })

  it('calls onCommit when Commit is clicked', async () => {
    const user = userEvent.setup()
    const onCommit = vi.fn()
    render(
      <TransactionGuardDialog
        open
        reason="switch-to-auto"
        pendingStatements={2}
        onCommit={onCommit}
        onRollback={vi.fn()}
        onOpenChange={vi.fn()}
      />,
    )
    await user.click(screen.getByRole('button', { name: /^commit$/i }))
    expect(onCommit).toHaveBeenCalled()
  })

  it('calls onRollback when Rollback is clicked', async () => {
    const user = userEvent.setup()
    const onRollback = vi.fn()
    render(
      <TransactionGuardDialog
        open
        reason="switch-to-auto"
        pendingStatements={2}
        onCommit={vi.fn()}
        onRollback={onRollback}
        onOpenChange={vi.fn()}
      />,
    )
    await user.click(screen.getByRole('button', { name: /^rollback$/i }))
    expect(onRollback).toHaveBeenCalled()
  })

  it('calls onOpenChange(false) when Cancel is clicked, without calling onCommit or onRollback', async () => {
    const user = userEvent.setup()
    const onOpenChange = vi.fn()
    const onCommit = vi.fn()
    const onRollback = vi.fn()
    render(
      <TransactionGuardDialog
        open
        reason="switch-to-auto"
        pendingStatements={2}
        onCommit={onCommit}
        onRollback={onRollback}
        onOpenChange={onOpenChange}
      />,
    )
    await user.click(screen.getByRole('button', { name: /cancel/i }))
    expect(onOpenChange).toHaveBeenCalledWith(false, expect.anything())
    expect(onCommit).not.toHaveBeenCalled()
    expect(onRollback).not.toHaveBeenCalled()
  })
})
