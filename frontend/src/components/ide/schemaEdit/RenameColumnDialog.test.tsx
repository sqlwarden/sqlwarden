import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { RenameColumnDialog } from './RenameColumnDialog'

describe('RenameColumnDialog', () => {
  it('preselects the current name for quick overtyping', () => {
    render(
      <RenameColumnDialog
        open
        onOpenChange={vi.fn()}
        tableName="orders"
        columnName="status"
        pending={false}
        onSubmit={vi.fn()}
      />,
    )
    expect(screen.getByLabelText(/new name for orders\.status/i)).toHaveValue('status')
  })

  it('rejects an empty name and the unchanged name', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(
      <RenameColumnDialog
        open
        onOpenChange={vi.fn()}
        tableName="orders"
        columnName="status"
        pending={false}
        onSubmit={onSubmit}
      />,
    )
    const input = screen.getByLabelText(/new name for orders\.status/i)
    await user.clear(input)
    expect(screen.getByRole('button', { name: /^rename$/i })).toBeDisabled()

    await user.type(input, 'status')
    expect(screen.getByText('Enter a different name.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^rename$/i })).toBeDisabled()

    await user.clear(input)
    await user.type(input, 'order_status')
    await user.click(screen.getByRole('button', { name: /^rename$/i }))
    expect(onSubmit).toHaveBeenCalledWith('order_status')
  })

  it('submits the trimmed new name', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(
      <RenameColumnDialog
        open
        onOpenChange={vi.fn()}
        tableName="orders"
        columnName="status"
        pending={false}
        onSubmit={onSubmit}
      />,
    )
    const input = screen.getByLabelText(/new name for orders\.status/i)
    await user.clear(input)
    await user.type(input, '  order_status  ')
    await user.click(screen.getByRole('button', { name: /^rename$/i }))
    expect(onSubmit).toHaveBeenCalledWith('order_status')
  })

  it('disables Cancel and Submit while pending', () => {
    render(
      <RenameColumnDialog
        open
        onOpenChange={vi.fn()}
        tableName="orders"
        columnName="status"
        pending
        onSubmit={vi.fn()}
      />,
    )
    expect(screen.getByRole('button', { name: /cancel/i })).toBeDisabled()
    expect(screen.getByRole('button', { name: /renaming/i })).toBeDisabled()
  })
})
