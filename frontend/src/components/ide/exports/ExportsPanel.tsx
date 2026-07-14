import { useState } from 'react'
import { queryKeys } from '#/lib/api/query-keys'
import { useQuery } from '@tanstack/react-query'
import { Button } from '#/components/ui/button'
import { Icon, type AppIcon } from '#/lib/icons'
import { cn } from '#/lib/utils'
import { getJobEvents } from '#/lib/api/exports'
import type { ExportJobOutput, JobRecord } from '#/lib/api/types'
import { Tip } from '../schema-diagram/Tip'
import { SidebarPane } from '../SidebarPane'
import type { IdeSidebarPanelProps } from '../ideActivities'
import { getExportRetryEntry } from './exportRetryCache'
import { useExportJobActions } from './useExportJobActions'
import { useExportJobs } from './useExportJobs'

export function ExportsPanel({ orgSlug, workspace }: IdeSidebarPanelProps) {
  const { jobs, isLoading, latestEventByJobId, refresh } = useExportJobs(orgSlug, workspace.id)
  const [expandedLogJobId, setExpandedLogJobId] = useState<string | null>(null)
  const actions = useExportJobActions(orgSlug, workspace, refresh)

  const visibleJobs = jobs.filter((job) => !actions.dismissed.has(job.id))

  return (
    <SidebarPane title="Exports" icon="download-01">
      {isLoading ? (
        <div className="px-3 py-4 text-center text-xs text-muted-foreground">Loading exports…</div>
      ) : visibleJobs.length === 0 ? (
        <div className="px-3 py-4 text-center text-xs text-muted-foreground">
          <p className="font-medium text-foreground">No exports yet</p>
          <p className="mt-0.5">Export a query from the toolbar or results pane to see it here.</p>
        </div>
      ) : (
        <div className="flex flex-col gap-1 p-2">
          {visibleJobs.map((job) => (
            <ExportJobRow
              key={job.id}
              job={job}
              latestEvent={latestEventByJobId.get(job.id)}
              canRetry={!!getExportRetryEntry(job.id)}
              sql={getExportRetryEntry(job.id)?.sql}
              logOpen={expandedLogJobId === job.id}
              onToggleLog={() =>
                setExpandedLogJobId((current) => (current === job.id ? null : job.id))
              }
              isDownloading={
                actions.download.isPending && actions.download.variables?.id === job.id
              }
              onDownload={() => actions.download.mutate(job)}
              onOpen={() => actions.openInEditor.mutate(job)}
              onReveal={() => actions.revealInFiles.mutate(job)}
              onRetry={() => actions.retry.mutate(job.id)}
              onCancel={() => actions.cancel.mutate(job.id)}
              onDismiss={() => actions.dismiss(job.id)}
              orgSlug={orgSlug}
              workspaceId={workspace.id}
            />
          ))}
        </div>
      )}
    </SidebarPane>
  )
}

function statusIconName(status: string): AppIcon {
  switch (status) {
    case 'succeeded':
      return 'checkmark-circle-02'
    case 'failed':
    case 'cancelled':
      return 'cancel-01'
    default:
      return 'loading-03' // queued | running
  }
}

function statusColorClass(status: string): string {
  switch (status) {
    case 'succeeded':
      return 'text-green-600 dark:text-green-400'
    case 'failed':
      return 'text-destructive'
    default:
      return 'text-muted-foreground'
  }
}

function statusLabel(status: string): string {
  return status.charAt(0).toUpperCase() + status.slice(1)
}

function formatLogTime(iso: string): string {
  return new Date(iso).toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

function ExportJobRow({
  job,
  latestEvent,
  canRetry,
  sql,
  logOpen,
  onToggleLog,
  isDownloading,
  onDownload,
  onOpen,
  onReveal,
  onRetry,
  onCancel,
  onDismiss,
  orgSlug,
  workspaceId,
}: {
  job: JobRecord
  latestEvent?: string
  canRetry: boolean
  sql?: string
  logOpen: boolean
  onToggleLog: () => void
  isDownloading: boolean
  onDownload: () => void
  onOpen: () => void
  onReveal: () => void
  onRetry: () => void
  onCancel: () => void
  onDismiss: () => void
  orgSlug: string
  workspaceId: number
}) {
  const output = job.output as ExportJobOutput | undefined
  const isRunning = job.status === 'running'
  const isFailedOrCancelled = job.status === 'failed' || job.status === 'cancelled'
  const isTerminal = job.status === 'succeeded' || isFailedOrCancelled

  return (
    <div className="flex flex-col gap-1.5 rounded-lg border border-border/60 bg-card/40 p-2.5 transition-colors hover:border-border hover:bg-muted/20">
      <div className="flex items-center gap-2">
        <Icon
          name={statusIconName(job.status)}
          size={13}
          className={cn('shrink-0', isRunning && 'animate-spin', statusColorClass(job.status))}
        />
        <span className="min-w-0 flex-1 truncate text-xs font-medium">
          {output?.filename ?? 'query-export.csv'}
        </span>
        <span className={cn('shrink-0 text-[10px] font-medium', statusColorClass(job.status))}>
          {statusLabel(job.status)}
        </span>
      </div>

      {sql && (
        <p className="truncate pl-5 font-mono text-[11px] text-muted-foreground" title={sql}>
          {sql.replace(/\s+/g, ' ').trim()}
        </p>
      )}

      {isRunning && latestEvent && (
        <p className="truncate pl-5 text-[11px] text-muted-foreground">{latestEvent}</p>
      )}
      {job.status === 'failed' && job.error_message && (
        <p className="truncate pl-5 text-[11px] text-destructive">{job.error_message}</p>
      )}

      {logOpen && <ExportJobLog orgSlug={orgSlug} workspaceId={workspaceId} jobId={job.id} />}

      <div className="flex items-center justify-end gap-1 pl-5">
        {isTerminal && (
          <Tip label={logOpen ? 'Hide log' : 'View log'}>
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label="Toggle export log"
              onClick={onToggleLog}
            >
              <Icon name="subject" size={12} />
            </Button>
          </Tip>
        )}
        {job.status === 'succeeded' && (
          <>
            <Tip label="Open">
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                aria-label="Open exported file"
                onClick={onOpen}
              >
                <Icon name="file-01" size={12} />
              </Button>
            </Tip>
            <Tip label="Reveal in Files">
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                aria-label="Reveal exported file in Files"
                onClick={onReveal}
              >
                <Icon name="folder-open" size={12} />
              </Button>
            </Tip>
            <Tip label={isDownloading ? 'Downloading…' : 'Download'}>
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                aria-label="Download export"
                disabled={isDownloading}
                onClick={onDownload}
              >
                <Icon
                  name={isDownloading ? 'loading-03' : 'download-01'}
                  size={12}
                  className={isDownloading ? 'animate-spin' : undefined}
                />
              </Button>
            </Tip>
          </>
        )}
        {job.status === 'failed' && canRetry && (
          <Tip label="Retry">
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label="Retry export"
              onClick={onRetry}
            >
              <Icon name="refresh" size={12} />
            </Button>
          </Tip>
        )}
        {(job.status === 'queued' || job.status === 'running') && (
          <Tip label="Cancel">
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label="Cancel export"
              onClick={onCancel}
            >
              <Icon name="cancel-01" size={12} />
            </Button>
          </Tip>
        )}
        {isTerminal && (
          <Tip label="Dismiss">
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label="Dismiss export"
              onClick={onDismiss}
            >
              <Icon name="delete-01" size={12} />
            </Button>
          </Tip>
        )}
      </div>
    </div>
  )
}

function ExportJobLog({
  orgSlug,
  workspaceId,
  jobId,
}: {
  orgSlug: string
  workspaceId: number
  jobId: string
}) {
  const { data, isLoading, isError } = useQuery({
    queryKey: queryKeys.exportJobLog(orgSlug, workspaceId, jobId),
    queryFn: () => getJobEvents(orgSlug, workspaceId, jobId),
  })

  if (isLoading) {
    return <p className="pl-5 text-[11px] text-muted-foreground">Loading log…</p>
  }
  if (isError) {
    return <p className="pl-5 text-[11px] text-destructive">Failed to load log.</p>
  }
  if (!data || data.items.length === 0) {
    return <p className="pl-5 text-[11px] text-muted-foreground">No log events.</p>
  }

  return (
    <div className="ml-5 flex flex-col gap-0.5 rounded-md border border-border/60 bg-muted/20 p-1.5">
      {data.items.map((event) => (
        <div key={event.id} className="flex items-baseline gap-1.5" title={event.message}>
          {event.created_at && (
            <span className="shrink-0 font-mono text-[10px] tabular-nums text-muted-foreground/70">
              {formatLogTime(event.created_at)}
            </span>
          )}
          <span
            className={cn(
              'min-w-0 flex-1 truncate text-[11px]',
              event.level === 'error'
                ? 'text-destructive'
                : event.level === 'warn'
                  ? 'text-amber-600 dark:text-amber-400'
                  : 'text-muted-foreground',
            )}
          >
            {event.message}
          </span>
        </div>
      ))}
    </div>
  )
}
