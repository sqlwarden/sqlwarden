import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '#/lib/api/errors'
import type { JobRecord } from '#/lib/api/types'
import { renderWithProviders } from '#/test/render'
import { ExportConfirmDialog } from './ExportConfirmDialog'
import { ExportToFilesDialog } from './ExportToFilesDialog'

const mocks = vi.hoisted(() => ({
  createExport: vi.fn(),
  rememberRetry: vi.fn(),
  toastSuccess: vi.fn(),
}))

vi.mock('#/lib/api/exports', () => ({ createExport: mocks.createExport }))
vi.mock('./exportRetryCache', () => ({ rememberExportRetry: mocks.rememberRetry }))
vi.mock('sonner', () => ({ toast: { success: mocks.toastSuccess } }))

const job: JobRecord = {
  id: 'job-1',
  type: 'export',
  visibility: 'private',
  status: 'queued',
  run_at: '',
  priority: 0,
  attempts: 0,
  max_attempts: 3,
  created_at: '',
  updated_at: '',
}

describe('ExportConfirmDialog', () => {
  it('confirms one statement and blocks multi-statement exports', async () => {
    const onConfirm = vi.fn()
    const onOpenChange = vi.fn()
    const user = userEvent.setup()
    const rendered = render(
      <ExportConfirmDialog open onOpenChange={onOpenChange} sql="select 1" onConfirm={onConfirm} />,
    )
    await user.click(screen.getByRole('button', { name: 'Export' }))
    expect(onConfirm).toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(onOpenChange).toHaveBeenCalledWith(false)

    rendered.rerender(
      <ExportConfirmDialog
        open
        onOpenChange={onOpenChange}
        sql="select 1; select 2"
        onConfirm={onConfirm}
      />,
    )
    expect(screen.getByText(/Multiple queries were selected/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Export' })).toBeDisabled()
  })
})

describe('ExportToFilesDialog', () => {
  beforeEach(() => {
    mocks.createExport.mockReset()
    mocks.rememberRetry.mockReset()
    mocks.toastSuccess.mockReset()
  })

  it('captures the query on open and queues an export with retry metadata', async () => {
    mocks.createExport.mockResolvedValue(job)
    const onOpenChange = vi.fn()
    const getSql = vi.fn(() => 'select * from users')
    const { user } = renderWithProviders(
      <ExportToFilesDialog
        open
        onOpenChange={onOpenChange}
        orgSlug="acme"
        workspaceId={3}
        connectionId={7}
        getSql={getSql}
      />,
    )

    expect(screen.getByText('select * from users')).toBeInTheDocument()
    await user.type(screen.getByLabelText('Filename'), ' users.csv ')
    await user.click(screen.getByRole('button', { name: 'Export' }))

    await waitFor(() =>
      expect(mocks.createExport).toHaveBeenCalledWith('acme', 3, 7, {
        sql: 'select * from users',
        format: 'csv',
        filename: 'users.csv',
      }),
    )
    expect(mocks.rememberRetry).toHaveBeenCalledWith('job-1', {
      connectionId: 7,
      sql: 'select * from users',
      filename: 'users.csv',
      format: 'csv',
    })
    expect(mocks.toastSuccess).toHaveBeenCalledWith('Export queued')
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it('shows server field errors and rejects multiple statements before submission', async () => {
    mocks.createExport.mockRejectedValue(
      new ApiError('Invalid export', 422, {
        fieldErrors: { filename: 'Filename is invalid.' },
      }),
    )
    const getSql = vi.fn(() => 'select 1')
    const { user, rerender } = renderWithProviders(
      <ExportToFilesDialog
        open
        onOpenChange={vi.fn()}
        orgSlug="acme"
        workspaceId={3}
        connectionId={7}
        getSql={getSql}
      />,
    )
    await user.click(screen.getByRole('button', { name: 'Export' }))
    expect(await screen.findByText('Filename is invalid.')).toBeInTheDocument()

    rerender(
      <ExportToFilesDialog
        open={false}
        onOpenChange={vi.fn()}
        orgSlug="acme"
        workspaceId={3}
        connectionId={7}
        getSql={() => 'select 1; select 2'}
      />,
    )
    rerender(
      <ExportToFilesDialog
        open
        onOpenChange={vi.fn()}
        orgSlug="acme"
        workspaceId={3}
        connectionId={7}
        getSql={() => 'select 1; select 2'}
      />,
    )
    expect(screen.getByText(/Multiple queries were selected/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Export' })).toBeDisabled()
  })
})
