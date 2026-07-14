import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '#/lib/api/errors'
import { useDownloadNow } from './useDownloadNow'

const mocks = vi.hoisted(() => ({
  downloadExport: vi.fn(),
  ensureSession: vi.fn(),
  saveBlobAs: vi.fn(),
  toastError: vi.fn(),
  toastInfo: vi.fn(),
}))

vi.mock('#/lib/api/exports', () => ({ downloadExport: mocks.downloadExport }))
vi.mock('../sessionErrors', () => ({ useEnsureSession: () => mocks.ensureSession }))
vi.mock('../saveFile', () => ({ saveBlobAs: mocks.saveBlobAs }))
vi.mock('sonner', () => ({ toast: { error: mocks.toastError, info: mocks.toastInfo } }))

describe('useDownloadNow', () => {
  beforeEach(() => {
    mocks.downloadExport.mockReset()
    mocks.ensureSession
      .mockReset()
      .mockImplementation(
        async (_connectionId: number, run: (sessionId: string) => Promise<unknown>) =>
          run('session-1'),
      )
    mocks.saveBlobAs.mockReset()
    mocks.toastError.mockReset()
    mocks.toastInfo.mockReset()
  })

  it('streams the response into a named download and resets progress', async () => {
    mocks.downloadExport.mockResolvedValue(
      new Response('id,name\n1,Ada\n', {
        headers: { 'Content-Disposition': 'attachment; filename="users.csv"' },
      }),
    )
    const { result } = renderHook(() => useDownloadNow('acme', 3))

    await act(async () => result.current.download(7, 'select * from users'))

    expect(mocks.downloadExport).toHaveBeenCalledWith(
      'acme',
      3,
      7,
      'session-1',
      { sql: 'select * from users', format: 'csv', filename: undefined },
      expect.any(AbortSignal),
    )
    expect(mocks.saveBlobAs).toHaveBeenCalledWith('users.csv', expect.any(Blob))
    expect(result.current).toEqual(
      expect.objectContaining({ isDownloading: false, bytesDownloaded: 0 }),
    )
  })

  it('aborts an active export and reports cancellation', async () => {
    mocks.downloadExport.mockImplementation(
      (
        _org: string,
        _workspace: number,
        _connection: number,
        _session: string,
        _input: unknown,
        signal: AbortSignal,
      ) =>
        new Promise((_resolve, reject) => {
          signal.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')))
        }),
    )
    const { result } = renderHook(() => useDownloadNow('acme', 3))

    let download: Promise<void>
    act(() => {
      download = result.current.download(7, 'select 1')
    })
    await waitFor(() => expect(result.current.isDownloading).toBe(true))
    act(() => result.current.cancel())
    await act(async () => download!)

    expect(mocks.toastInfo).toHaveBeenCalledWith('Export cancelled.')
    expect(result.current.isDownloading).toBe(false)
  })

  it('shows the backend message for API failures', async () => {
    mocks.downloadExport.mockRejectedValue(new ApiError('Export is not allowed.', 403))
    const { result } = renderHook(() => useDownloadNow('acme', 3))

    await act(async () => result.current.download(7, 'select 1'))

    expect(mocks.toastError).toHaveBeenCalledWith('Export is not allowed.')
    expect(mocks.saveBlobAs).not.toHaveBeenCalled()
  })
})
