import { useEffect, useId, useRef, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api } from '#/lib/api/client'
import { errorMessage, isApiError } from '#/lib/api/errors'
import { queryKeys } from '#/lib/api/query-keys'
import type { Workspace } from '#/lib/api/types'
import { Button } from '#/components/ui/button'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '#/components/ui/dialog'
import { Input } from '#/components/ui/input'
import { Label } from '#/components/ui/label'

type WorkspaceFieldErrors = { name?: string; description?: string }

export function CreateWorkspaceDialog({
  orgSlug,
  open,
  onOpenChange,
  onCreated,
}: {
  orgSlug: string
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: (workspace: Workspace) => void | Promise<void>
}) {
  const queryClient = useQueryClient()
  const nameId = useId()
  const descriptionId = useId()
  const nameRef = useRef<HTMLInputElement>(null)
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [fieldErrors, setFieldErrors] = useState<WorkspaceFieldErrors>({})

  useEffect(() => {
    if (!open) return
    const timeout = window.setTimeout(() => nameRef.current?.focus(), 0)
    return () => window.clearTimeout(timeout)
  }, [open])

  const createWorkspace = useMutation({
    mutationFn: () =>
      api.post<Workspace>(`/api/v1/orgs/${orgSlug}/workspaces`, {
        name: name.trim(),
        description: description.trim(),
      }),
    onSuccess: async (workspace) => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.orgWorkspacesScope(orgSlug) })
      toast.success('Workspace created')
      onOpenChange(false)
      setName('')
      setDescription('')
      setFieldErrors({})
      await onCreated(workspace)
    },
    onError: (error) => {
      if (isApiError(error)) {
        const next = {
          name: error.fieldErrors?.name,
          description: error.fieldErrors?.description,
        }
        setFieldErrors(next)
        if (next.name || next.description) return
      }
      toast.error(errorMessage(error, 'Failed to create workspace'))
    },
  })

  function setDialogOpen(nextOpen: boolean) {
    if (!nextOpen && createWorkspace.isPending) return
    onOpenChange(nextOpen)
    if (!nextOpen) {
      setName('')
      setDescription('')
      setFieldErrors({})
    }
  }

  function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!name.trim()) {
      setFieldErrors({ name: 'Workspace name is required.' })
      nameRef.current?.focus()
      return
    }
    setFieldErrors({})
    createWorkspace.mutate()
  }

  return (
    <Dialog open={open} onOpenChange={setDialogOpen}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Create workspace</DialogTitle>
          <DialogDescription>
            Group related database connections, saved queries, and environments.
          </DialogDescription>
        </DialogHeader>
        <form className="mt-6 flex flex-col gap-4" onSubmit={submit}>
          <div className="flex flex-col gap-2">
            <Label htmlFor={nameId}>Name</Label>
            <Input
              ref={nameRef}
              id={nameId}
              value={name}
              onChange={(event) => {
                setName(event.target.value)
                setFieldErrors((current) => ({ ...current, name: undefined }))
              }}
              aria-invalid={fieldErrors.name ? true : undefined}
              aria-describedby={fieldErrors.name ? `${nameId}-error` : undefined}
              disabled={createWorkspace.isPending}
              autoComplete="off"
            />
            {fieldErrors.name ? (
              <p id={`${nameId}-error`} className="text-sm text-destructive">
                {fieldErrors.name}
              </p>
            ) : null}
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor={descriptionId}>Description (optional)</Label>
            <Input
              id={descriptionId}
              value={description}
              onChange={(event) => {
                setDescription(event.target.value)
                setFieldErrors((current) => ({ ...current, description: undefined }))
              }}
              aria-invalid={fieldErrors.description ? true : undefined}
              aria-describedby={fieldErrors.description ? `${descriptionId}-error` : undefined}
              disabled={createWorkspace.isPending}
            />
            {fieldErrors.description ? (
              <p id={`${descriptionId}-error`} className="text-sm text-destructive">
                {fieldErrors.description}
              </p>
            ) : null}
          </div>
          <DialogFooter>
            <DialogClose
              render={<Button type="button" variant="ghost" disabled={createWorkspace.isPending} />}
            >
              Cancel
            </DialogClose>
            <Button type="submit" disabled={createWorkspace.isPending}>
              {createWorkspace.isPending ? 'Creating…' : 'Create workspace'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
