import { useCallback, useEffect, useRef, useState } from 'react'
import { toast } from 'sonner'
import { ApiError } from '#/lib/api/errors'
import { downloadExport } from '#/lib/api/exports'
import { useEnsureSession } from '../sessionErrors'
import { saveBlobAs } from '../saveFile'

export interface DownloadNowState {
  isDownloading: boolean
  bytesDownloaded: number
}

const FILENAME_FROM_DISPOSITION = /filename="?([^";]+)"?/

export function useDownloadNow(orgSlug: string, workspaceId: number) {
  const [state, setState] = useState<DownloadNowState>({ isDownloading: false, bytesDownloaded: 0 })
  const controllerRef = useRef<AbortController | null>(null)
  const ensureSession = useEnsureSession(orgSlug, workspaceId)

  useEffect(() => {
    if (!state.isDownloading) return
    function warnBeforeUnload(e: BeforeUnloadEvent) {
      e.preventDefault()
      e.returnValue = ''
    }
    window.addEventListener('beforeunload', warnBeforeUnload)
    return () => window.removeEventListener('beforeunload', warnBeforeUnload)
  }, [state.isDownloading])

  const download = useCallback(
    async (connectionId: number, sql: string, filename?: string) => {
      const controller = new AbortController()
      controllerRef.current = controller
      setState({ isDownloading: true, bytesDownloaded: 0 })
      try {
        const response = await ensureSession(
          connectionId,
          (sessionId) => downloadExport(orgSlug, workspaceId, connectionId, sessionId, { sql, format: 'csv', filename }, controller.signal),
          controller.signal,
        )

        const disposition = response.headers.get('Content-Disposition') ?? ''
        const resolvedFilename = FILENAME_FROM_DISPOSITION.exec(disposition)?.[1] ?? 'query-export.csv'

        const reader = response.body?.getReader()
        if (!reader) throw new Error('Export response has no body')
        const chunks: Uint8Array[] = []
        let total = 0
        for (;;) {
          const { done, value } = await reader.read()
          if (done) break
          chunks.push(value)
          total += value.byteLength
          setState({ isDownloading: true, bytesDownloaded: total })
        }

        saveBlobAs(resolvedFilename, new Blob(chunks as BlobPart[], { type: 'text/csv;charset=utf-8' }))
      } catch (err) {
        if (err instanceof DOMException && err.name === 'AbortError') {
          toast.info('Export cancelled.')
        } else if (err instanceof ApiError) {
          toast.error(err.message)
        } else {
          // The response already started (200 + headers sent) before the
          // backend aborted the connection mid-stream, so a byte-limit
          // overflow surfaces here as a generic network failure, not an
          // ApiError — see the sync truncation fix in handlers_exports.go.
          toast.error('Export exceeded the size limit — try Export to workspace instead.')
        }
      } finally {
        controllerRef.current = null
        setState({ isDownloading: false, bytesDownloaded: 0 })
      }
    },
    [ensureSession, orgSlug, workspaceId],
  )

  const cancel = useCallback(() => {
    controllerRef.current?.abort()
  }, [])

  return { ...state, download, cancel }
}
