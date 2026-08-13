import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { CreateTableDialog } from './CreateTableDialog'

/** The submit button is disabled while the form is invalid, so validation
 *  feedback is only reachable via the form's own submit event (Enter key in
 *  the browser) rather than a click on a disabled control. The dialog renders
 *  through a portal, so the form lives outside the render() container. */
function submitForm() {
  const form = document.querySelector('form')
  if (!form) throw new Error('form not found')
  fireEvent.submit(form)
}

const scope = [{ kind: 'schema', name: 'public' }]

describe('CreateTableDialog', () => {
  it('starts with a single empty column row using the first advertised column type', () => {
    render(
      <CreateTableDialog
        open
        onOpenChange={vi.fn()}
        scope={scope}
        columnTypes={['text', 'integer']}
        pending={false}
        onSubmit={vi.fn()}
      />,
    )
    expect(screen.getAllByLabelText('Column name')).toHaveLength(1)
    expect(screen.getByLabelText('Column type')).toHaveTextContent('text')
  })

  it('keeps submit disabled and reports a table name is required once touched', async () => {
    const onSubmit = vi.fn()
    render(
      <CreateTableDialog
        open
        onOpenChange={vi.fn()}
        scope={scope}
        columnTypes={['text', 'integer']}
        pending={false}
        onSubmit={onSubmit}
      />,
    )
    expect(screen.getByRole('button', { name: /create table/i })).toBeDisabled()
    submitForm()
    expect(onSubmit).not.toHaveBeenCalled()
    expect(screen.getByText('Table name is required.')).toBeInTheDocument()
  })

  it('rejects duplicate column names, case-insensitively', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(
      <CreateTableDialog
        open
        onOpenChange={vi.fn()}
        scope={scope}
        columnTypes={['text', 'integer']}
        pending={false}
        onSubmit={onSubmit}
      />,
    )
    await user.type(screen.getByLabelText('Table name'), 'orders')
    await user.type(screen.getAllByLabelText('Column name')[0], 'ID')
    await user.click(screen.getByRole('button', { name: /add column/i }))
    await user.type(screen.getAllByLabelText('Column name')[1], 'id')
    submitForm()

    expect(onSubmit).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: /create table/i })).toBeDisabled()
    expect(screen.getByText('Column names must be unique.')).toBeInTheDocument()
  })

  it('submits the trimmed table name with the built column payload', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(
      <CreateTableDialog
        open
        onOpenChange={vi.fn()}
        scope={scope}
        columnTypes={['text', 'integer']}
        pending={false}
        onSubmit={onSubmit}
      />,
    )
    await user.type(screen.getByLabelText('Table name'), '  orders  ')
    await user.type(screen.getAllByLabelText('Column name')[0], 'id')
    await user.click(screen.getByRole('checkbox', { name: 'PK' }))
    await user.click(screen.getByRole('button', { name: /create table/i }))

    expect(onSubmit).toHaveBeenCalledWith('orders', [
      { name: 'id', data_type: 'text', nullable: false, primary_key: true },
    ])
  })

  it('keeps Cancel and Submit disabled while pending', () => {
    render(
      <CreateTableDialog
        open
        onOpenChange={vi.fn()}
        scope={scope}
        columnTypes={['text']}
        pending
        onSubmit={vi.fn()}
      />,
    )
    expect(screen.getByRole('button', { name: /cancel/i })).toBeDisabled()
    expect(screen.getByRole('button', { name: /creating/i })).toBeDisabled()
  })

  it('removes a column row but keeps at least one', async () => {
    const user = userEvent.setup()
    render(
      <CreateTableDialog
        open
        onOpenChange={vi.fn()}
        scope={scope}
        columnTypes={['text']}
        pending={false}
        onSubmit={vi.fn()}
      />,
    )
    await user.click(screen.getByRole('button', { name: /add column/i }))
    expect(screen.getAllByLabelText('Column name')).toHaveLength(2)
    const removeButtons = screen.getAllByLabelText('Remove column')
    await user.click(removeButtons[1])
    expect(screen.getAllByLabelText('Column name')).toHaveLength(1)
    await user.click(screen.getAllByLabelText('Remove column')[0])
    expect(screen.getAllByLabelText('Column name')).toHaveLength(1)
  })
})
