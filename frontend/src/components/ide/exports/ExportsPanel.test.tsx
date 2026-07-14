import { QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { JobRecord, Workspace } from '#/lib/api/types'
import { createTestQueryClient } from '#/test/render'
import { ExportsPanel } from './ExportsPanel'

const mocks = vi.hoisted(() => ({
  cancel: vi.fn(),
  dismiss: vi.fn(),
  download: vi.fn(),
  open: vi.fn(),
  retry: vi.fn(),
  reveal: vi.fn(),
  useExportJobs: vi.fn(),
}))

vi.mock('./useExportJobs', () => ({ useExportJobs: mocks.useExportJobs }))
vi.mock('./useExportJobActions', () => ({
  useExportJobActions: () => ({
    cancel: { mutate: mocks.cancel },
    dismissed: new Set<string>(),
    dismiss: mocks.dismiss,
    download: { isPending: false, mutate: mocks.download },
    openInEditor: { mutate: mocks.open },
    retry: { mutate: mocks.retry },
    revealInFiles: { mutate: mocks.reveal },
  }),
}))

const workspace: Workspace = {
  id: 3,
  org_id: 1,
  owner_type: 'org',
  owner_id: 1,
  name: 'Analytics',
  environment_count: 0,
  connection_count: 1,
  created_at: '',
  updated_at: '',
}

function job(id: string, status: JobRecord['status']): JobRecord {
  return {
    id,
    type: 'export_query_csv',
    visibility: 'private',
    status,
    run_at: '',
    priority: 0,
    attempts: 0,
    max_attempts: 1,
    error_message: status === 'failed' ? 'Connection lost' : undefined,
    output:
      status === 'succeeded'
        ? { file_id: 42, filename: 'orders.csv', format: 'csv', row_count: 2, byte_count: 20 }
        : undefined,
    created_at: '',
    updated_at: '',
  }
}

describe('ExportsPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  function renderPanel() {
    return render(
      <QueryClientProvider client={createTestQueryClient()}>
        <ExportsPanel orgSlug="acme" workspace={workspace} />
      </QueryClientProvider>,
    )
  }

  it('renders loading and empty states', () => {
    mocks.useExportJobs.mockReturnValue({
      jobs: [],
      isLoading: true,
      latestEventByJobId: new Map(),
      refresh: vi.fn(),
    })
    const view = renderPanel()
    expect(screen.getByText('Loading exports…')).toBeInTheDocument()

    mocks.useExportJobs.mockReturnValue({
      jobs: [],
      isLoading: false,
      latestEventByJobId: new Map(),
      refresh: vi.fn(),
    })
    view.rerender(
      <QueryClientProvider client={createTestQueryClient()}>
        <ExportsPanel orgSlug="acme" workspace={workspace} />
      </QueryClientProvider>,
    )
    expect(screen.getByText('No exports yet')).toBeInTheDocument()
  })

  it('shows running progress and allows cancellation', () => {
    const running = job('running', 'running')
    mocks.useExportJobs.mockReturnValue({
      jobs: [running],
      isLoading: false,
      latestEventByJobId: new Map([['running', 'Reading rows']]),
      refresh: vi.fn(),
    })
    renderPanel()

    expect(screen.getByText('Reading rows')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Cancel export' }))
    expect(mocks.cancel).toHaveBeenCalledWith('running')
  })

  it('exposes completed export actions and failed job details', () => {
    const succeeded = job('success', 'succeeded')
    const failed = job('failed', 'failed')
    mocks.useExportJobs.mockReturnValue({
      jobs: [succeeded, failed],
      isLoading: false,
      latestEventByJobId: new Map(),
      refresh: vi.fn(),
    })
    renderPanel()

    expect(screen.getByText('orders.csv')).toBeInTheDocument()
    expect(screen.getByText('Connection lost')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Open exported file' }))
    fireEvent.click(screen.getByRole('button', { name: 'Reveal exported file in Files' }))
    fireEvent.click(screen.getByRole('button', { name: 'Download export' }))
    expect(mocks.open).toHaveBeenCalledWith(succeeded)
    expect(mocks.reveal).toHaveBeenCalledWith(succeeded)
    expect(mocks.download).toHaveBeenCalledWith(succeeded)
  })
})
