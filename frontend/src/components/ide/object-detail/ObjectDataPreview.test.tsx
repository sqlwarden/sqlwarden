import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { createTestQueryClient } from '#/test/render'
import { ObjectDataPreview } from './ObjectDataPreview'
import type { ObjectViewModel } from './registry'

const dataGridSpy = vi.fn()
vi.mock('../dataGrid/DataGrid', () => ({
  DataGrid: (props: unknown) => {
    dataGridSpy(props)
    return <div data-testid="data-grid-stub" />
  },
}))

vi.mock('#/lib/api/query', async (importOriginal) => {
  const actual = await importOriginal<typeof import('#/lib/api/query')>()
  return {
    ...actual,
    runConnectionQuery: vi.fn(() =>
      Promise.resolve({
        columns: [{ name: 'id', type: 'integer', raw_type: 'integer', nullable: false }],
        rows: [[{ type: 'integer', integer: 1 }]],
        exhausted: true,
      }),
    ),
    fetchConnectionCursorPage: vi.fn(),
  }
})

function buildVm(): ObjectViewModel {
  return {
    orgSlug: 'acme',
    workspaceId: 1,
    connectionId: 7,
    sessionId: 'session-1',
    dialect: {
      previewQuery: () => 'select * from t',
      boundedCountQuery: () => 'select count(*) from t',
      exactCountQuery: () => 'select count(*) from t',
    },
    detail: { ref: { schema: 'public', name: 't', kind: 'table' } },
  } as unknown as ObjectViewModel
}

describe('ObjectDataPreview', () => {
  it('renders fetched rows through DataGrid with paging wired to the infinite query', async () => {
    const queryClient = createTestQueryClient()
    render(
      <QueryClientProvider client={queryClient}>
        <ObjectDataPreview vm={buildVm()} />
      </QueryClientProvider>,
    )

    expect(await screen.findByTestId('data-grid-stub')).toBeInTheDocument()
    const props = dataGridSpy.mock.calls.at(-1)?.[0] as {
      columns: unknown[]
      rows: unknown[]
      onScrollNearEnd: () => void
      isLoadingMore: boolean
    }
    expect(props.columns).toEqual([
      { name: 'id', type: 'integer', raw_type: 'integer', nullable: false },
    ])
    expect(props.rows).toEqual([[{ type: 'integer', integer: 1 }]])
    expect(props.isLoadingMore).toBe(false)
    expect(typeof props.onScrollNearEnd).toBe('function')
  })
})
