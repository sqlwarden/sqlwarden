import { useState } from 'react'
import { Button } from '#/components/ui/button'
import { Checkbox } from '#/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '#/components/ui/dialog'
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from '#/components/ui/field'
import { Input } from '#/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '#/components/ui/select'
import type {
  DbColumn,
  ObjectRef,
  SchemaEditColumnChanges,
  SchemaEditRequest,
  SchemaEditSpec,
} from '#/lib/api/types'
import { ColumnTypeInput } from './ColumnTypeInput'
import { canonicalColumnType } from './columnTypes'

const defaultModes = [
  { value: 'keep', label: 'Keep current default' },
  { value: 'set', label: 'Set expression' },
  { value: 'remove', label: 'Remove default' },
]

export function ColumnEditDialog({
  objectRef,
  column,
  editor,
  pending,
  onClose,
  onSubmit,
}: {
  objectRef: ObjectRef
  column?: DbColumn
  editor: SchemaEditSpec
  pending: boolean
  onClose: () => void
  onSubmit: (request: SchemaEditRequest) => void
}) {
  const [name, setName] = useState(column?.name ?? '')
  const [dataType, setDataType] = useState(column?.data_type ?? editor.column_types[0] ?? '')
  const [nullable, setNullable] = useState(column?.nullable ?? true)
  const [defaultMode, setDefaultMode] = useState(column ? 'keep' : 'set')
  const [defaultValue, setDefaultValue] = useState(column?.default ?? '')
  const [error, setError] = useState<string | null>(null)
  const title = column ? 'Alter column' : 'Add column'

  function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (pending) return
    const canonical = canonicalColumnType(
      dataType,
      editor.column_types,
      editor.parameterized_column_types,
    )
    let message: string | null = null
    if (!name.trim()) message = 'Column name is required.'
    else if ((!column || dataType !== column.data_type) && !canonical)
      message = 'Enter a supported column type.'
    else if (
      editor.supports_column_defaults &&
      column &&
      defaultMode === 'set' &&
      !defaultValue.trim()
    )
      message = 'Enter a default expression, or choose Remove default.'
    if (message) {
      setError(message)
      event.currentTarget
        .querySelector<HTMLElement>(
          !name.trim()
            ? '#edit-column-name'
            : !canonical
              ? '#edit-column-type'
              : '#edit-column-default',
        )
        ?.focus()
      return
    }
    if (column) {
      const changes: SchemaEditColumnChanges = {}
      if (dataType !== column.data_type && canonical) changes.data_type = canonical
      if (nullable !== column.nullable) changes.nullable = nullable
      if (editor.supports_column_defaults && defaultMode !== 'keep')
        changes.default = defaultMode === 'remove' ? 'NULL' : defaultValue.trim()
      if (Object.keys(changes).length === 0) {
        setError('Change a column property before saving.')
        return
      }
      onSubmit({ operation: 'alter_column', ref: objectRef, name: column.name, changes })
    } else {
      onSubmit({
        operation: 'add_column',
        ref: objectRef,
        column: {
          name: name.trim(),
          data_type: canonical!,
          nullable,
          primary_key: false,
          ...(editor.supports_column_defaults && defaultValue.trim()
            ? { default: defaultValue.trim() }
            : {}),
        },
      })
    }
    setError(null)
  }

  return (
    <Dialog open onOpenChange={(open) => !open && !pending && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>In {objectRef.name}</DialogDescription>
        </DialogHeader>
        <form onSubmit={submit} className="flex flex-col gap-4">
          <FieldGroup>
            <Field data-invalid={Boolean(error && !name.trim())}>
              <FieldLabel htmlFor="edit-column-name">Column name</FieldLabel>
              <Input
                id="edit-column-name"
                value={name}
                onChange={(event) => setName(event.target.value)}
                disabled={pending || Boolean(column)}
                autoComplete="off"
                aria-invalid={Boolean(error && !name.trim())}
                aria-describedby={error ? 'column-edit-error' : undefined}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="edit-column-type">Column type</FieldLabel>
              <ColumnTypeInput
                id="edit-column-type"
                value={dataType}
                onChange={setDataType}
                columnTypes={editor.column_types}
                rules={editor.parameterized_column_types}
                disabled={pending}
              />
              {column && (
                <FieldDescription>
                  Leave the type unchanged to preserve its current definition.
                </FieldDescription>
              )}
            </Field>
            <Field orientation="horizontal">
              <Checkbox
                id="edit-column-nullable"
                checked={nullable}
                onCheckedChange={(checked) => setNullable(Boolean(checked))}
                disabled={pending}
              />
              <FieldLabel htmlFor="edit-column-nullable">Allow null values</FieldLabel>
            </Field>
            {editor.supports_column_defaults && (
              <>
                {column && (
                  <Field>
                    <FieldLabel htmlFor="edit-column-default-mode">Default</FieldLabel>
                    <Select
                      items={defaultModes}
                      value={defaultMode}
                      onValueChange={(value) => value && setDefaultMode(value)}
                      disabled={pending}
                    >
                      <SelectTrigger id="edit-column-default-mode">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectGroup>
                          {defaultModes.map((mode) => (
                            <SelectItem key={mode.value} value={mode.value}>
                              {mode.label}
                            </SelectItem>
                          ))}
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                    <FieldDescription>Current default: {column.default || 'None'}</FieldDescription>
                  </Field>
                )}
                {defaultMode === 'set' && (
                  <Field data-invalid={Boolean(error && column && !defaultValue.trim())}>
                    <FieldLabel htmlFor="edit-column-default">Default expression</FieldLabel>
                    <Input
                      id="edit-column-default"
                      value={defaultValue}
                      onChange={(event) => setDefaultValue(event.target.value)}
                      disabled={pending}
                      placeholder={column ? undefined : 'No default'}
                      autoComplete="off"
                      aria-invalid={Boolean(error && column && !defaultValue.trim())}
                      aria-describedby={error ? 'column-edit-error' : undefined}
                    />
                    <FieldDescription>
                      Use a SQL expression. Enclose text values in single quotes.
                    </FieldDescription>
                  </Field>
                )}
              </>
            )}
          </FieldGroup>
          {error && (
            <FieldError id="column-edit-error" role="alert">
              {error}
            </FieldError>
          )}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose} disabled={pending}>
              Cancel
            </Button>
            <Button type="submit" disabled={pending}>
              {column ? 'Save changes' : 'Add column'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
