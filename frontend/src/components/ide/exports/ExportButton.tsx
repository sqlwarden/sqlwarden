import { useState } from 'react'
import { Button } from '#/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '#/components/ui/dropdown-menu'
import { Icon } from '#/lib/icons'
import { cn } from '#/lib/utils'
import { Tip } from '../schema-diagram/Tip'
import { ExportToFilesDialog } from './ExportToFilesDialog'
import { formatBytes } from './formatBytes'
import { useDownloadNow } from './useDownloadNow'

type ExportButtonProps = {
  orgSlug: string
  workspaceId: number
  connectionId: number | undefined
  getSql: () => string
  className?: string
}

export function ExportButton({ orgSlug, workspaceId, connectionId, getSql, className }: ExportButtonProps) {
  const [dialogOpen, setDialogOpen] = useState(false)
  const downloadNow = useDownloadNow(orgSlug, workspaceId)

  const disabled = !connectionId
  const hint = downloadNow.isDownloading
    ? 'Exporting — canceling stops the export.'
    : 'Export the query results as a file.'

  function handleDownloadNowClick() {
    if (!connectionId) return
    const sql = getSql()
    if (!sql) return
    void downloadNow.download(connectionId, sql)
  }

  return (
    <>
      <div className={cn('flex items-stretch', className)}>
        <Tip label={hint}>
          <Button
            type="button"
            variant="outline"
            className="rounded-r-none border-r-0 px-2.5"
            disabled={disabled || downloadNow.isDownloading}
            onClick={handleDownloadNowClick}
          >
            <Icon
              name={downloadNow.isDownloading ? 'loading-03' : 'download-01'}
              size={13}
              data-icon="inline-start"
              className={downloadNow.isDownloading ? 'animate-spin' : undefined}
            />
            {downloadNow.isDownloading ? `Exporting… ${formatBytes(downloadNow.bytesDownloaded)}` : 'Export'}
          </Button>
        </Tip>

        {downloadNow.isDownloading ? (
          <Tip label="Cancel export">
            <Button
              type="button"
              variant="outline"
              className="rounded-l-none px-2"
              aria-label="Cancel export"
              onClick={downloadNow.cancel}
            >
              <Icon name="cancel-01" size={13} />
            </Button>
          </Tip>
        ) : (
          <DropdownMenu>
            <DropdownMenuTrigger
              render={
                <Button
                  type="button"
                  variant="outline"
                  className="rounded-l-none border-l border-l-border px-1.5"
                  aria-label="More export options"
                  disabled={disabled}
                />
              }
            >
              <Icon name="chevron-down" size={12} />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onClick={() => setDialogOpen(true)}>
                <Icon name="folder" size={13} data-icon="inline-start" />
                Export to workspace
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        )}
      </div>

      {connectionId && dialogOpen && (
        <ExportToFilesDialog
          open
          onOpenChange={setDialogOpen}
          orgSlug={orgSlug}
          workspaceId={workspaceId}
          connectionId={connectionId}
          getSql={getSql}
        />
      )}
    </>
  )
}
