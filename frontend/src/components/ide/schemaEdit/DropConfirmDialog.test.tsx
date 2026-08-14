import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { DropConfirmDialog } from './DropConfirmDialog'

describe('DropConfirmDialog', () => {
  it('names the object type and name in the confirmation title', () => {
    render(
      <DropConfirmDialog
        open
        onOpenChange={vi.fn()}
        objectType="table"
        objectName="orders"
        description="This permanently drops the table orders."
        cascadeAvailable={false}
        pending={false}
        onConfirm={vi.fn()}
      />,
    )
    expect(screen.getByRole('alertdialog')).toHaveTextContent('Drop table')
    expect(screen.getByRole('alertdialog')).toHaveTextContent('orders')
  })

  it('omits the cascade option when the driver does not support it', () => {
    render(
      <DropConfirmDialog
        open
        onOpenChange={vi.fn()}
        objectType="column"
        objectName="status"
        description="This permanently drops the column."
        cascadeAvailable={false}
        pending={false}
        onConfirm={vi.fn()}
      />,
    )
    expect(screen.queryByRole('checkbox', { name: /cascade/i })).not.toBeInTheDocument()
  })

  it('confirms with cascade=false by default and cascade=true once checked', async () => {
    const user = userEvent.setup()
    const onConfirm = vi.fn()
    render(
      <DropConfirmDialog
        open
        onOpenChange={vi.fn()}
        objectType="schema"
        objectName="public"
        description="This permanently drops the schema."
        cascadeAvailable
        pending={false}
        onConfirm={onConfirm}
      />,
    )
    await user.click(screen.getByRole('button', { name: /^drop$/i }))
    expect(onConfirm).toHaveBeenLastCalledWith(false)

    await user.click(screen.getByRole('checkbox', { name: /cascade/i }))
    await user.click(screen.getByRole('button', { name: /^drop$/i }))
    expect(onConfirm).toHaveBeenLastCalledWith(true)
  })

  it('resets the cascade checkbox each time the dialog opens', () => {
    const { rerender } = render(
      <DropConfirmDialog
        open={false}
        onOpenChange={vi.fn()}
        objectType="schema"
        objectName="public"
        description="This permanently drops the schema."
        cascadeAvailable
        pending={false}
        onConfirm={vi.fn()}
      />,
    )
    rerender(
      <DropConfirmDialog
        open
        onOpenChange={vi.fn()}
        objectType="schema"
        objectName="public"
        description="This permanently drops the schema."
        cascadeAvailable
        pending={false}
        onConfirm={vi.fn()}
      />,
    )
    expect(screen.getByRole('checkbox', { name: /cascade/i })).not.toBeChecked()
  })

  it('disables Cancel and Confirm while pending', () => {
    render(
      <DropConfirmDialog
        open
        onOpenChange={vi.fn()}
        objectType="table"
        objectName="orders"
        description="This permanently drops the table orders."
        cascadeAvailable={false}
        pending
        onConfirm={vi.fn()}
      />,
    )
    expect(screen.getByRole('button', { name: /cancel/i })).toBeDisabled()
    expect(screen.getByRole('button', { name: /dropping/i })).toBeDisabled()
  })
})
