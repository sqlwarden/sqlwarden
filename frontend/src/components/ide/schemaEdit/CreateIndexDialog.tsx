import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
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
import { orgConnectionObjectQueryOptions } from '#/lib/api/query'
import type { ObjectRef, SchemaEditIndexColumn, SchemaEditRequest } from '#/lib/api/types'
import { useEvictGoneSession } from '../sessionErrors'

export function CreateIndexDialog({
  objectRef,
  orgSlug,
  workspaceId,
  connectionId,
  sessionId,
  pending,
  onClose,
  onSubmit,
}: {
  objectRef: ObjectRef
  orgSlug: string
  workspaceId: number
  connectionId: number
  sessionId?: string
  pending: boolean
  onClose: () => void
  onSubmit: (request: SchemaEditRequest) => void
}) {
  const detail = useQuery(
    orgConnectionObjectQueryOptions(orgSlug, workspaceId, connectionId, sessionId, objectRef),
  )
  useEvictGoneSession(connectionId, [detail.error])
  const columns = detail.data?.relational?.columns ?? []
  const [name, setName] = useState('')
  const [unique, setUnique] = useState(false)
  const [rows, setRows] = useState<(SchemaEditIndexColumn & { id: number })[]>([
    { id: 0, name: '' },
  ])
  const [nextId, setNextId] = useState(1)
  const [error, setError] = useState<string | null>(null)

  function updateRow(id: number, patch: Partial<SchemaEditIndexColumn>) {
    setRows((current) => current.map((row) => (row.id === id ? { ...row, ...patch } : row)))
  }
  function moveRow(index: number, offset: number) {
    setRows((current) => {
      const next = [...current]
      ;[next[index], next[index + offset]] = [next[index + offset], next[index]]
      return next
    })
  }
  function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (pending) return
    if (!name.trim()) {
      setError('Index name is required.')
      event.currentTarget.querySelector<HTMLElement>('#create-index-name')?.focus()
      return
    }
    const invalid = rows.findIndex((row) => !columns.some((column) => column.name === row.name))
    if (invalid >= 0 || new Set(rows.map((row) => row.name)).size !== rows.length) {
      setError('Choose a different table column for each index position.')
      event.currentTarget
        .querySelector<HTMLElement>('#index-column-' + rows[Math.max(invalid, 0)].id)
        ?.focus()
      return
    }
    setError(null)
    onSubmit({
      operation: 'create_index',
      ref: objectRef,
      name: name.trim(),
      unique,
      index_columns: rows.map(({ name: columnName, descending }) => ({
        name: columnName,
        descending: Boolean(descending),
      })),
    })
  }

  return (
    <Dialog open onOpenChange={(open) => !open && !pending && onClose()}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>Create index</DialogTitle>
          <DialogDescription>On {objectRef.name}</DialogDescription>
        </DialogHeader>
        <form onSubmit={submit} className="flex flex-col gap-4">
          <FieldGroup>
            <Field data-invalid={Boolean(error && !name.trim())}>
              <FieldLabel htmlFor="create-index-name">Index name</FieldLabel>
              <Input
                id="create-index-name"
                value={name}
                onChange={(event) => setName(event.target.value)}
                disabled={pending}
                autoComplete="off"
                aria-invalid={Boolean(error && !name.trim())}
                aria-describedby={error ? 'create-index-error' : undefined}
              />
            </Field>
            <Field orientation="horizontal">
              <Checkbox
                id="create-index-unique"
                checked={unique}
                onCheckedChange={(checked) => setUnique(Boolean(checked))}
                disabled={pending}
              />
              <FieldLabel htmlFor="create-index-unique">Unique index</FieldLabel>
            </Field>
            {detail.isLoading ? (
              <p role="status">Loading columns…</p>
            ) : detail.isError ? (
              <Field>
                <FieldError>Could not load table columns.</FieldError>
                <Button type="button" variant="outline" onClick={() => detail.refetch()}>
                  Retry
                </Button>
              </Field>
            ) : columns.length === 0 ? (
              <FieldDescription>This table has no available columns.</FieldDescription>
            ) : (
              <>
                <FieldDescription>Index columns are used in the order shown.</FieldDescription>
                {rows.map((row, index) => (
                  <Field key={row.id}>
                    <FieldLabel htmlFor={'index-column-' + row.id}>Column {index + 1}</FieldLabel>
                    <div className="flex flex-wrap items-center gap-2">
                      <Select
                        items={columns.map((column) => ({
                          value: column.name,
                          label: column.name,
                        }))}
                        value={row.name || null}
                        onValueChange={(value) => value && updateRow(row.id, { name: value })}
                        disabled={pending}
                      >
                        <SelectTrigger
                          id={'index-column-' + row.id}
                          className="min-w-32 flex-1"
                          aria-invalid={Boolean(error && !row.name)}
                          aria-describedby={error ? 'create-index-error' : undefined}
                        >
                          <SelectValue placeholder="Choose column" />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectGroup>
                            {columns.map((column) => (
                              <SelectItem
                                key={column.name}
                                value={column.name}
                                disabled={rows.some(
                                  (other) => other.id !== row.id && other.name === column.name,
                                )}
                              >
                                {column.name}
                              </SelectItem>
                            ))}
                          </SelectGroup>
                        </SelectContent>
                      </Select>
                      <Field orientation="horizontal" className="w-auto">
                        <Checkbox
                          id={'index-desc-' + row.id}
                          checked={Boolean(row.descending)}
                          onCheckedChange={(checked) =>
                            updateRow(row.id, { descending: Boolean(checked) })
                          }
                          disabled={pending}
                        />
                        <FieldLabel htmlFor={'index-desc-' + row.id}>Descending</FieldLabel>
                      </Field>
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        aria-label={'Move column ' + (index + 1) + ' up'}
                        disabled={pending || index === 0}
                        onClick={() => moveRow(index, -1)}
                      >
                        Up
                      </Button>
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        aria-label={'Move column ' + (index + 1) + ' down'}
                        disabled={pending || index === rows.length - 1}
                        onClick={() => moveRow(index, 1)}
                      >
                        Down
                      </Button>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        aria-label={'Remove column ' + (index + 1)}
                        disabled={pending || rows.length === 1}
                        onClick={() =>
                          setRows((current) => current.filter((other) => other.id !== row.id))
                        }
                      >
                        Remove
                      </Button>
                    </div>
                  </Field>
                ))}
                <Button
                  type="button"
                  variant="outline"
                  disabled={pending || rows.length >= columns.length}
                  onClick={() => {
                    setRows((current) => [...current, { id: nextId, name: '' }])
                    setNextId((current) => current + 1)
                  }}
                >
                  Add column
                </Button>
              </>
            )}
          </FieldGroup>
          {error && (
            <FieldError id="create-index-error" role="alert">
              {error}
            </FieldError>
          )}
          <DialogFooter>
            <Button type="button" variant="outline" disabled={pending} onClick={onClose}>
              Cancel
            </Button>
            <Button
              type="submit"
              disabled={pending || detail.isLoading || detail.isError || columns.length === 0}
            >
              Create index
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
