import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { ColumnEditDialog } from './ColumnEditDialog'
import { editor } from './schemaEdit.fixtures'

const objectRef = { scope: [{ kind: 'schema', name: 'HR' }], kind: 'table', name: 'ORDERS' }

describe('ColumnEditDialog', () => {
  it('adds a custom type and SQL default', async () => {
    const onSubmit = vi.fn()
    render(
      <ColumnEditDialog
        objectRef={objectRef}
        editor={editor}
        pending={false}
        onClose={vi.fn()}
        onSubmit={onSubmit}
      />,
    )
    fireEvent.change(screen.getByLabelText('Column name'), { target: { value: 'AMOUNT' } })
    await userEvent.click(screen.getByLabelText('Column type'))
    await userEvent.click(await screen.findByRole('option', { name: 'Custom type…' }))
    fireEvent.change(screen.getByLabelText('Custom column type'), {
      target: { value: 'number(10, 2)' },
    })
    fireEvent.change(screen.getByLabelText('Default expression'), { target: { value: '0' } })
    await userEvent.click(screen.getByRole('button', { name: 'Add column' }))
    expect(onSubmit).toHaveBeenCalledWith({
      operation: 'add_column',
      ref: objectRef,
      column: {
        name: 'AMOUNT',
        data_type: 'NUMBER(10,2)',
        nullable: true,
        primary_key: false,
        default: '0',
      },
    })
  })
  it('preserves nullability and default when changing only the type', async () => {
    const onSubmit = vi.fn()
    render(
      <ColumnEditDialog
        objectRef={objectRef}
        column={{
          name: 'AMOUNT',
          data_type: 'NUMBER(10,2)',
          nullable: false,
          default: '0',
          ordinal: 1,
        }}
        editor={editor}
        pending={false}
        onClose={vi.fn()}
        onSubmit={onSubmit}
      />,
    )
    fireEvent.change(screen.getByLabelText('Custom column type'), {
      target: { value: 'NUMBER(12,2)' },
    })
    await userEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    expect(onSubmit).toHaveBeenCalledWith({
      operation: 'alter_column',
      ref: objectRef,
      name: 'AMOUNT',
      changes: { data_type: 'NUMBER(12,2)' },
    })
  })
  it('can remove a default without changing an existing type outside the editable palette', async () => {
    const onSubmit = vi.fn()
    render(
      <ColumnEditDialog
        objectRef={objectRef}
        column={{
          name: 'VALUE',
          data_type: 'CUSTOM_TYPE',
          nullable: true,
          default: 'NULL',
          ordinal: 1,
        }}
        editor={editor}
        pending={false}
        onClose={vi.fn()}
        onSubmit={onSubmit}
      />,
    )
    await userEvent.click(screen.getByLabelText('Default', { exact: true }))
    await userEvent.click(await screen.findByRole('option', { name: 'Remove default' }))
    await userEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    expect(onSubmit).toHaveBeenCalledWith({
      operation: 'alter_column',
      ref: objectRef,
      name: 'VALUE',
      changes: { default: 'NULL' },
    })
  })
  it('selects presets with the keyboard and can return from custom type entry', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(
      <ColumnEditDialog
        objectRef={objectRef}
        editor={editor}
        pending={false}
        onClose={vi.fn()}
        onSubmit={onSubmit}
      />,
    )
    fireEvent.change(screen.getByLabelText('Column name'), { target: { value: 'TITLE' } })
    await user.click(screen.getByLabelText('Column type'))
    await screen.findByRole('option', { name: 'NUMBER' })
    await user.keyboard('{Home}{ArrowDown}{Enter}')
    expect(screen.getByLabelText('Column type')).toHaveTextContent('VARCHAR2(255)')
    await user.click(screen.getByLabelText('Column type'))
    await user.click(await screen.findByRole('option', { name: 'Custom type…' }))
    fireEvent.change(screen.getByLabelText('Custom column type'), {
      target: { value: 'NUMBER(500)' },
    })
    expect(screen.getByLabelText('Custom column type')).toHaveAttribute('aria-invalid', 'true')
    await user.click(screen.getByLabelText('Column type'))
    await user.click(await screen.findByRole('option', { name: 'NUMBER' }))
    expect(screen.queryByLabelText('Custom column type')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Add column' }))
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        column: expect.objectContaining({ name: 'TITLE', data_type: 'NUMBER' }),
      }),
    )
  })

  it('rejects missing names and retains the form for correction', async () => {
    const onSubmit = vi.fn()
    render(
      <ColumnEditDialog
        objectRef={objectRef}
        editor={editor}
        pending={false}
        onClose={vi.fn()}
        onSubmit={onSubmit}
      />,
    )
    await userEvent.click(screen.getByRole('button', { name: 'Add column' }))
    expect(onSubmit).not.toHaveBeenCalled()
    expect(screen.getByRole('alert')).toHaveTextContent('Column name is required.')
    expect(screen.getByLabelText('Column name')).toHaveFocus()
  })
})
