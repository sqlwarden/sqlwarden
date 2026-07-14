import { useEffect, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { getPrivateWorkspaceFileContent } from '#/lib/api/files'
import { cancelExportJob, createExport } from '#/lib/api/exports'
import { orgWorkspacePrivateFileBrowserQueryOptions } from '#/lib/api/query'
import type { ExportJobOutput, JobRecord, Workspace } from '#/lib/api/types'
import { saveTextAs } from '../saveFile'
import { newFileTab, useIde } from '../useIdeStore'
import { dismissExport, getDismissedExportIds } from './dismissedExports'
import { getExportRetryEntry, rememberExportRetry } from './exportRetryCache'

export function useExportJobActions(orgSlug: string, workspace: Workspace, refresh: () => void) {
  const queryClient = useQueryClient()
  const openTab = useIde((state) => state.openTab)
  const setNodeExpanded = useIde((state) => state.setNodeExpanded)
  const setActiveActivity = useIde((state) => state.setActiveActivity)
  const [dismissed, setDismissed] = useState(() => getDismissedExportIds(workspace.id))

  useEffect(() => {
    setDismissed(getDismissedExportIds(workspace.id))
  }, [workspace.id])

  const cancel = useMutation({
    mutationFn: (jobId: string) => cancelExportJob(orgSlug, workspace.id, jobId),
    onSuccess: refresh,
    onError: () => toast.error('Failed to cancel export.'),
  })

  const retry = useMutation({
    mutationFn: async (jobId: string) => {
      const entry = getExportRetryEntry(jobId)
      if (!entry) throw new Error('missing_retry_entry')
      const job = await createExport(orgSlug, workspace.id, entry.connectionId, {
        sql: entry.sql,
        format: entry.format,
        filename: entry.filename || undefined,
      })
      rememberExportRetry(job.id, entry)
      return job
    },
    onSuccess: () => {
      toast.success('Export queued')
      refresh()
    },
    onError: () => toast.error('Failed to retry export.'),
  })

  const download = useMutation({
    mutationFn: async (job: JobRecord) => {
      const output = job.output as ExportJobOutput
      const { text } = await getPrivateWorkspaceFileContent(orgSlug, workspace.id, output.file_id)
      saveTextAs(output.filename, text)
    },
    onError: () => toast.error('Failed to save file.'),
  })

  async function resolveExportFile(job: JobRecord) {
    const output = job.output as ExportJobOutput
    const result = await queryClient.fetchQuery(
      orgWorkspacePrivateFileBrowserQueryOptions(orgSlug, workspace.id, output.file_id),
    )
    if (!result.file) throw new Error('export_file_not_found')
    return result
  }

  const openInEditor = useMutation({
    mutationFn: async (job: JobRecord) => {
      const result = await resolveExportFile(job)
      openTab(newFileTab(result.file!, workspace))
    },
    onError: () => toast.error('Failed to open file — it may have been deleted from Files.'),
  })

  const revealInFiles = useMutation({
    mutationFn: async (job: JobRecord) => {
      const result = await resolveExportFile(job)
      for (const segment of result.path) {
        if (segment.object_type === 'folder') setNodeExpanded(`folder:${segment.id}`, true)
      }
      setActiveActivity('files')
      openTab(newFileTab(result.file!, workspace))
    },
    onError: () => toast.error('Failed to open file — it may have been deleted from Files.'),
  })

  function dismiss(jobId: string) {
    dismissExport(workspace.id, jobId)
    setDismissed((current) => new Set(current).add(jobId))
  }

  return { cancel, dismissed, dismiss, download, openInEditor, retry, revealInFiles }
}
