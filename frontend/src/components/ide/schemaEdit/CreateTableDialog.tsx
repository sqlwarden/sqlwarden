import { useEffect, useRef, useState } from 'react'
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
import { Field, FieldError, FieldGroup, FieldLabel } from '#/components/ui/field'
import { Icon } from '#/lib/icons'
import { Input } from '#/components/ui/input'
import { Label } from '#/components/ui/label'
import { ColumnTypeInput } from './ColumnTypeInput'
import { canonicalColumnType } from './columnTypes'
import { scopeLabel } from '#/lib/api/scope'
import type { ScopePath, SchemaEditColumn, ParameterizedColumnType } from '#/lib/api/types'

type ColumnRow = {
  id: number
  name: string
  dataType: string
  nullable: boolean
  primaryKey: boolean
  defaultValue: string
}

let nextRowId = 0
function emptyRow(dataType: string): ColumnRow {
  nextRowId += 1
  return { id: nextRowId, name: '', dataType, nullable: true, primaryKey: false, defaultValue: '' }
}

export type CreateTableDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  scope: ScopePath
  columnTypes: string[]
  parameterizedColumnTypes?: ParameterizedColumnType[]
  supportsColumnDefaults?: boolean
  pending: boolean
  onSubmit: (name: string, columns: SchemaEditColumn[]) => void
}

/** Validated column rows ready to submit, or null while the form is incomplete. */
function toValidColumns(rows: ColumnRow[]): SchemaEditColumn[] | null {
  const trimmed = rows.map((r) => ({ ...r, name: r.name.trim() }))
  if (trimmed.length === 0 || trimmed.some((r) => r.name === '')) return null
  const seen = new Set<string>()
  for (const row of trimmed) {
    const folded = row.name.toLowerCase()
    if (seen.has(folded)) return null
    seen.add(folded)
  }
  return trimmed.map((r) => ({
    name: r.name,
    data_type: r.dataType,
    nullable: r.primaryKey ? false : r.nullable,
    primary_key: r.primaryKey,
    ...(r.defaultValue.trim() ? { default: r.defaultValue.trim() } : {}),
  }))
}

/** Create table dialog: table name plus at least one column, each with a
 *  name, a data type validated against the driver's advertised type grammar,
 *  nullability, and primary key. */
export function CreateTableDialog({
  open,
  onOpenChange,
  scope,
  columnTypes,
  parameterizedColumnTypes = [],
  supportsColumnDefaults = false,
  pending,
  onSubmit,
}: CreateTableDialogProps) {
  const [name, setName] = useState('')
  const [rows, setRows] = useState<ColumnRow[]>([])
  const [touched, setTouched] = useState(false)
  const nameRef = useRef<HTMLInputElement>(null)
  const lastRowNameRef = useRef<HTMLInputElement>(null)
  const focusNextRow = useRef(false)

  useEffect(() => {
    if (!open) return
    setName('')
    setRows([emptyRow(columnTypes[0] ?? '')])
    setTouched(false)
    const timer = setTimeout(() => nameRef.current?.focus(), 50)
    return () => clearTimeout(timer)
    // eslint-disable-next-line react-hooks/exhaustive-deps -- reset only when the dialog opens
  }, [open])

  useEffect(() => {
    if (focusNextRow.current) {
      focusNextRow.current = false
      lastRowNameRef.current?.focus()
    }
  }, [rows])

  const trimmedName = name.trim()
  const columns = toValidColumns(rows)
  const nameError = touched && trimmedName === '' ? 'Table name is required.' : null
  const duplicateColumns =
    touched && columns === null && rows.every((r) => r.name.trim() !== '')
      ? 'Column names must be unique.'
      : null
  const invalidTypes = rows.some(
    (row) => canonicalColumnType(row.dataType, columnTypes, parameterizedColumnTypes) === null,
  )
  const disabled = pending || trimmedName === '' || columns === null || invalidTypes

  function updateRow(id: number, patch: Partial<ColumnRow>) {
    setRows((current) => current.map((r) => (r.id === id ? { ...r, ...patch } : r)))
  }

  function addRow() {
    focusNextRow.current = true
    setRows((current) => [...current, emptyRow(columnTypes[0] ?? '')])
  }

  function removeRow(id: number) {
    setRows((current) => (current.length > 1 ? current.filter((r) => r.id !== id) : current))
  }

  function requestClose() {
    if (!pending) onOpenChange(false)
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setTouched(true)
    if (disabled) return
    onSubmit(
      trimmedName,
      columns!.map((column) => ({
        ...column,
        data_type: canonicalColumnType(column.data_type, columnTypes, parameterizedColumnTypes)!,
      })),
    )
  }

  return (
    <Dialog open={open} onOpenChange={(next) => (next || !pending) && onOpenChange(next)}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Create table</DialogTitle>
          <DialogDescription>In {scopeLabel(scope) || 'this scope'}</DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <FieldGroup>
            <Field data-invalid={Boolean(nameError)}>
              <FieldLabel htmlFor="create-table-name">Table name</FieldLabel>
              <Input
                id="create-table-name"
                ref={nameRef}
                value={name}
                disabled={pending}
                onChange={(e) => setName(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Escape' && !pending) {
                    e.preventDefault()
                    requestClose()
                  }
                }}
                autoComplete="off"
                aria-invalid={Boolean(nameError) || undefined}
              />
              {nameError && <FieldError>{nameError}</FieldError>}
            </Field>
          </FieldGroup>

          <div className="flex flex-col gap-2">
            <div className="flex items-center justify-between">
              <Label>Columns</Label>
              <Button type="button" variant="outline" size="sm" disabled={pending} onClick={addRow}>
                <Icon name="plus-sign" size={12} data-icon="inline-start" />
                Add column
              </Button>
            </div>
            <div className="flex flex-col gap-2">
              {rows.map((row) => (
                <FieldGroup key={row.id}>
                  <div className="flex flex-wrap items-start gap-2">
                    <Input
                      ref={row.id === rows[rows.length - 1].id ? lastRowNameRef : undefined}
                      value={row.name}
                      disabled={pending}
                      onChange={(e) => updateRow(row.id, { name: e.target.value })}
                      placeholder="column name"
                      autoComplete="off"
                      aria-label="Column name"
                      className="min-w-0 flex-1"
                    />
                    <div className="w-48 shrink-0">
                      <ColumnTypeInput
                        value={row.dataType}
                        onChange={(dataType) => updateRow(row.id, { dataType })}
                        columnTypes={columnTypes}
                        rules={parameterizedColumnTypes}
                        disabled={pending}
                      />
                    </div>
                    <label className="flex shrink-0 items-center gap-1.5 text-xs text-muted-foreground">
                      <Checkbox
                        checked={row.primaryKey ? false : row.nullable}
                        disabled={pending || row.primaryKey}
                        onCheckedChange={(checked) =>
                          updateRow(row.id, { nullable: Boolean(checked) })
                        }
                      />
                      Null
                    </label>
                    <label className="flex shrink-0 items-center gap-1.5 text-xs text-muted-foreground">
                      <Checkbox
                        checked={row.primaryKey}
                        disabled={pending}
                        onCheckedChange={(checked) =>
                          updateRow(row.id, {
                            primaryKey: Boolean(checked),
                            nullable: checked ? false : row.nullable,
                          })
                        }
                      />
                      PK
                    </label>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon-sm"
                      disabled={pending || rows.length <= 1}
                      aria-label="Remove column"
                      onClick={() => removeRow(row.id)}
                    >
                      <Icon name="delete-02" size={13} />
                    </Button>
                  </div>
                  {supportsColumnDefaults && (
                    <Field>
                      <FieldLabel htmlFor={'column-default-' + row.id}>
                        Default expression
                      </FieldLabel>
                      <Input
                        id={'column-default-' + row.id}
                        value={row.defaultValue}
                        onChange={(event) =>
                          updateRow(row.id, { defaultValue: event.target.value })
                        }
                        disabled={pending}
                        placeholder="No default"
                        autoComplete="off"
                      />
                    </Field>
                  )}
                </FieldGroup>
              ))}
            </div>
            {duplicateColumns && <p className="text-xs text-destructive">{duplicateColumns}</p>}
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" disabled={pending} onClick={requestClose}>
              Cancel
            </Button>
            <Button type="submit" disabled={disabled}>
              {pending ? 'Creating…' : 'Create table'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
