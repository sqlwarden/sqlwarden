import type { Connection } from '#/lib/api/types'
import { Button } from '#/components/ui/button'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '#/components/ui/dialog'
import { Input } from '#/components/ui/input'
import { driverBrands } from './connection-drivers/index'
import { TestStatusIndicator } from './ConnectionTestStatus'
import { DriverBadge } from './DriverBadge'
import { DriverFields, FormField } from './ConnectionFormFields'
import { useEditConnectionForm } from './useEditConnectionForm'

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  orgSlug: string
  workspaceId: number
  connection: Connection | undefined
  canRevealDsn: boolean
}

export function EditConnectionDialog({
  open,
  onOpenChange,
  orgSlug,
  workspaceId,
  connection,
  canRevealDsn,
}: Props) {
  const form = useEditConnectionForm({
    open,
    onOpenChange,
    orgSlug,
    workspaceId,
    connection,
    canRevealDsn,
  })

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    form.submit()
  }
  const isPending = form.updateConnection.isPending
  const fieldsDisabled = isPending || form.revealDsnPending

  return (
    <Dialog open={open} onOpenChange={form.handleOpenChange}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>Edit Connection</DialogTitle>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="flex flex-col">
          <div className="flex max-h-[min(560px,calc(100svh-14rem))] flex-col gap-4 overflow-y-auto pb-1">
            <div className="flex items-center gap-3 rounded-lg border border-border bg-muted/40 p-3">
              <DriverBadge driver={form.driver.id} size="md" className="size-8 shrink-0" />
              <div className="min-w-0 flex-1">
                <div className="text-xs font-medium text-foreground">{form.driver.label}</div>
                <div className="truncate text-[10px] text-muted-foreground">
                  {driverBrands[form.driver.id]?.description}
                </div>
              </div>
            </div>

            <p className="text-xs text-muted-foreground">
              {form.revealDsnAllowed
                ? 'Connection credentials are shown below because you can manage this connection.'
                : "For security, connection credentials are never shown after they're saved. Re-enter the full connection details below to update this connection."}
            </p>

            <div className="grid grid-cols-6 gap-3">
              <div className="col-span-6">
                <FormField label="Name" error={form.errors.name}>
                  <Input
                    value={form.name}
                    disabled={isPending}
                    placeholder={`My ${form.driver.label}`}
                    aria-invalid={form.errors.name ? true : undefined}
                    onChange={(e) => form.changeName(e.target.value)}
                  />
                </FormField>
              </div>

              <DriverFields
                driver={form.driver}
                values={form.fields}
                errors={form.errors.fields}
                disabled={fieldsDisabled}
                onChange={form.changeField}
                scopeDiscovery={form.scopeDiscovery}
                defaultScope={form.defaultScope}
                onDatabaseChange={form.selectDatabase}
                onSchemaChange={form.selectSchema}
              />
            </div>

            {form.errors._form ? (
              <p className="text-xs text-destructive">{form.errors._form}</p>
            ) : null}

            {form.conflict ? (
              <div className="flex flex-col gap-2 rounded-lg border border-destructive/40 bg-destructive/5 p-3">
                <p className="text-xs text-destructive">
                  This connection has active sessions. Rotating the credentials will drop them.
                </p>
                <Button
                  type="button"
                  variant="destructive"
                  size="sm"
                  className="self-start"
                  disabled={isPending}
                  onClick={() => form.submit(true)}
                >
                  Rotate anyway
                </Button>
              </div>
            ) : null}
          </div>

          <DialogFooter className="mt-4 items-center gap-3 border-t border-border/60 pt-4 sm:justify-between">
            <div className="flex min-w-0 items-center gap-3">
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={!form.requiredFieldsFilled || form.testConnection.isPending || isPending}
                onClick={() => void form.testConnection.mutateAsync().catch(() => {})}
              >
                {form.testConnection.isPending ? 'Testing…' : 'Test Connection'}
              </Button>
              <TestStatusIndicator state={form.testState} />
            </div>
            <div className="flex items-center gap-2">
              <DialogClose render={<Button type="button" variant="ghost" disabled={isPending} />}>
                Cancel
              </DialogClose>
              <Button type="submit" disabled={isPending}>
                {isPending ? 'Saving…' : 'Save'}
              </Button>
            </div>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
