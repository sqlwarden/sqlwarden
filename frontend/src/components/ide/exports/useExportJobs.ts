import { useMemo, useRef } from 'react'
import { queryKeys } from '#/lib/api/query-keys'
import { useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import { orgWorkspaceJobsQueryOptions } from '#/lib/api/query'
import { getJobEvents } from '#/lib/api/exports'
import type { JobEventPage, JobRecord } from '#/lib/api/types'

export const EXPORT_JOB_TYPE = 'export_query_csv'
const POLL_INTERVAL_MS = 3000

export function isTerminalJobStatus(status: string): boolean {
  return status === 'succeeded' || status === 'failed' || status === 'cancelled'
}

export interface EventCursor {
  afterId?: string
  lastMessage?: string
}

/** Folds one polled events page into a job's running cursor: advances
 *  afterId past everything the page returned and keeps the newest message,
 *  so the panel shows only the latest line without re-fetching history. */
export function nextEventCursor(current: EventCursor, page: JobEventPage): EventCursor {
  if (page.items.length === 0) return current
  return {
    afterId: page.next_after_id ?? current.afterId,
    lastMessage: page.items[page.items.length - 1].message,
  }
}

export function useExportJobs(orgSlug: string, workspaceId: number) {
  const queryClient = useQueryClient()
  const cursors = useRef(new Map<string, EventCursor>())

  const jobsQuery = useQuery({
    ...orgWorkspaceJobsQueryOptions(orgSlug, workspaceId, { page_size: 50, sort: 'created_at', order: 'desc' }),
    refetchInterval: (query) => {
      const items = query.state.data?.items ?? []
      const nonTerminal = items.some((j) => j.type === EXPORT_JOB_TYPE && !isTerminalJobStatus(j.status))
      return nonTerminal ? POLL_INTERVAL_MS : false
    },
  })

  const exportJobs = useMemo(
    () => (jobsQuery.data?.items ?? []).filter((j) => j.type === EXPORT_JOB_TYPE),
    [jobsQuery.data],
  )
  const runningJobIds = useMemo(
    () => exportJobs.filter((j) => j.status === 'running').map((j) => j.id),
    [exportJobs],
  )

  const eventQueries = useQueries({
    queries: runningJobIds.map((jobId) => ({
      queryKey: queryKeys.exportJobLatestEvent(orgSlug, workspaceId, jobId),
      queryFn: async () => {
        const prev = cursors.current.get(jobId) ?? {}
        const page = await getJobEvents(orgSlug, workspaceId, jobId, prev.afterId)
        const next = nextEventCursor(prev, page)
        cursors.current.set(jobId, next)
        return next.lastMessage
      },
      refetchInterval: POLL_INTERVAL_MS,
    })),
  })

  const latestEventByJobId = new Map<string, string>()
  runningJobIds.forEach((jobId, i) => {
    const message = eventQueries[i]?.data
    if (message) latestEventByJobId.set(jobId, message)
  })

  function refresh() {
    queryClient.invalidateQueries({ queryKey: queryKeys.orgWorkspaceJobsScope(orgSlug, workspaceId) })
  }

  return {
    jobs: exportJobs as JobRecord[],
    isLoading: jobsQuery.isLoading,
    latestEventByJobId,
    refresh,
  }
}
