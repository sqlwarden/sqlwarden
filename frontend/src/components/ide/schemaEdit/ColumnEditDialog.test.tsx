import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { SchemaEditSpec } from '#/lib/api/types'
import { ColumnEditDialog } from './ColumnEditDialog'
import { editor } from './schemaEdit.fixtures'

const objectRef = { scope: [{ kind: 'schema', name: 'HR' }], kind: 'table', name: 'ORDERS' }

// Mirrors the shape internal/engine/engines/mysql/buildMySQLDDLSpec emits for
// integer types: the bare name plus modifier permutations as fixed entries and
// a display-width rule per suffix.
const mysqlEditor: SchemaEditSpec = {
  ...editor,
  column_types: ['int', 'smallint', 'smallint unsigned', 'smallint unsigned zerofill'],
  parameterized_column_types: [
    { name: 'smallint', parameters: [{ name: 'width', min: 1, max: 255 }] },
    { name: 'smallint', suffix: 'unsigned', parameters: [{ name: 'width', min: 1, max: 255 }] },
  ],
}

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
  it('edits a MySQL "smallint unsigned" column without flagging the type as unsupported', async () => {
    const onSubmit = vi.fn()
    render(
      <ColumnEditDialog
        objectRef={objectRef}
        column={{ name: 'QTY', data_type: 'smallint unsigned', nullable: true, ordinal: 1 }}
        editor={mysqlEditor}
        pending={false}
        onClose={vi.fn()}
        onSubmit={onSubmit}
      />,
    )
    expect(screen.getByLabelText('Column type')).toHaveTextContent('smallint unsigned')
    expect(screen.queryByText('Enter a supported data type.')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('checkbox', { name: 'Allow null values' }))
    await userEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    expect(onSubmit).toHaveBeenCalledWith({
      operation: 'alter_column',
      ref: objectRef,
      name: 'QTY',
      changes: { nullable: false },
    })
  })

  it('filters the type list by typed text and selects a match', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(
      <ColumnEditDialog
        objectRef={objectRef}
        column={{ name: 'QTY', data_type: 'int', nullable: true, ordinal: 1 }}
        editor={mysqlEditor}
        pending={false}
        onClose={vi.fn()}
        onSubmit={onSubmit}
      />,
    )
    await user.click(screen.getByLabelText('Column type'))
    await user.keyboard('unsigned zero')
    expect(screen.queryByRole('option', { name: 'int' })).not.toBeInTheDocument()
    await user.click(await screen.findByRole('option', { name: 'smallint unsigned zerofill' }))
    expect(screen.getByLabelText('Column type')).toHaveTextContent('smallint unsigned zerofill')
    await user.click(screen.getByRole('button', { name: 'Save changes' }))
    expect(onSubmit).toHaveBeenCalledWith({
      operation: 'alter_column',
      ref: objectRef,
      name: 'QTY',
      changes: { data_type: 'smallint unsigned zerofill' },
    })
  })

  it('keeps a rule-matched "smallint(5) unsigned" type selectable after switching away and back', async () => {
    const user = userEvent.setup()
    render(
      <ColumnEditDialog
        objectRef={objectRef}
        column={{ name: 'QTY', data_type: 'smallint(5) unsigned', nullable: true, ordinal: 1 }}
        editor={mysqlEditor}
        pending={false}
        onClose={vi.fn()}
        onSubmit={vi.fn()}
      />,
    )
    expect(screen.queryByText('Enter a supported data type.')).not.toBeInTheDocument()
    await user.click(screen.getByLabelText('Column type'))
    await user.click(await screen.findByRole('option', { name: 'int' }))
    expect(screen.getByLabelText('Column type')).toHaveTextContent('int')
    await user.click(screen.getByLabelText('Column type'))
    expect(await screen.findByRole('option', { name: 'smallint(5) unsigned' })).toBeInTheDocument()
    await user.click(screen.getByRole('option', { name: 'smallint(5) unsigned' }))
    expect(screen.getByLabelText('Column type')).toHaveTextContent('smallint(5) unsigned')
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
