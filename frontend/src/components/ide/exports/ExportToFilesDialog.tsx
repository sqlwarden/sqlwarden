import { useEffect, useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Button } from '#/components/ui/button'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '#/components/ui/dialog'
import { Input } from '#/components/ui/input'
import { Label } from '#/components/ui/label'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '#/components/ui/select'
import { ApiError } from '#/lib/api/errors'
import { createExport } from '#/lib/api/exports'
import { EXPORT_FORMATS } from './exportFormats'
import { rememberExportRetry } from './exportRetryCache'
import { countSqlStatements } from '../sqlStatements'

type ExportToFilesDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  orgSlug: string
  workspaceId: number
  connectionId: number
  getSql: () => string
}

export function ExportToFilesDialog({ open, onOpenChange, orgSlug, workspaceId, connectionId, getSql }: ExportToFilesDialogProps) {
  const [filename, setFilename] = useState('')
  const [format, setFormat] = useState(EXPORT_FORMATS[0].value)
  const [fieldError, setFieldError] = useState<string | null>(null)
  // Captured once when the dialog opens so it stays stable while it's open,
  // even if the editor selection changes underneath it.
  const [sql, setSql] = useState('')

  useEffect(() => {
    if (open) {
      setFilename('')
      setFormat(EXPORT_FORMATS[0].value)
      setFieldError(null)
      setSql(getSql())
    }
    // getSql is a fresh closure on every render — only re-run when the dialog opens.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  const isMultiStatement = countSqlStatements(sql) > 1

  const submit = useMutation({
    mutationFn: async () => {
      const trimmedFilename = filename.trim()
      const job = await createExport(orgSlug, workspaceId, connectionId, { sql, format, filename: trimmedFilename || undefined })
      return { job, sql, filename: trimmedFilename }
    },
    onSuccess: ({ job, sql, filename: submittedFilename }) => {
      rememberExportRetry(job.id, { connectionId, sql, filename: submittedFilename, format })
      toast.success('Export queued')
      onOpenChange(false)
    },
    onError: (err) => {
      if (err instanceof ApiError && (err.fieldErrors?.sql || err.fieldErrors?.filename)) {
        setFieldError(err.fieldErrors.sql ?? err.fieldErrors.filename)
      } else if (err instanceof ApiError) {
        setFieldError(err.message)
      } else {
        setFieldError('Something went wrong. Please try again.')
      }
    },
  })

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setFieldError(null)
    submit.mutate()
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>Export to workspace</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <p className="text-xs text-muted-foreground">
            Runs this query and saves the result as a file in this workspace's Files, so you can pick it up later.
          </p>

          <div className="flex flex-col gap-1.5">
            <Label>Query</Label>
            <pre className="max-h-32 overflow-auto rounded-md border border-border bg-muted/40 p-2 font-mono text-[11px] whitespace-pre-wrap text-foreground">
              {sql}
            </pre>
            {isMultiStatement && (
              <p className="text-xs text-destructive">
                Multiple queries were selected. Only a single query can be exported right now — select just the query you want.
              </p>
            )}
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor="export-filename">Filename</Label>
            <Input
              id="export-filename"
              value={filename}
              onChange={(e) => { setFilename(e.target.value); setFieldError(null) }}
              placeholder="query-export-20260705-142233"
              autoComplete="off"
              aria-invalid={!!fieldError}
            />
          </div>

          <div className="flex flex-col gap-1.5">
            <Label>Format</Label>
            <Select
              items={EXPORT_FORMATS.map((f) => ({ label: f.label, value: f.value, disabled: !f.enabled }))}
              value={format}
              onValueChange={(value) => { if (value) setFormat(value) }}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {EXPORT_FORMATS.map((f) => (
                    <SelectItem key={f.value} value={f.value} disabled={!f.enabled}>
                      {f.label}{!f.enabled ? ' (coming soon)' : ''}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </div>

          {fieldError && <p className="text-xs text-destructive">{fieldError}</p>}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={submit.isPending || isMultiStatement}>
              {submit.isPending ? 'Queuing…' : 'Export'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
