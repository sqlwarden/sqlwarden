import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { CreateIndexDialog } from './CreateIndexDialog'

const queryFn = vi.hoisted(() =>
  vi.fn(async () => ({ relational: { columns: [{ name: 'ID' }, { name: 'LABEL' }] } })),
)
vi.mock('#/lib/api/query', () => ({
  orgConnectionObjectQueryOptions: () => ({ queryKey: ['index-table'], queryFn }),
}))
vi.mock('../sessionErrors', () => ({ useEvictGoneSession: vi.fn() }))
const objectRef = { scope: [{ kind: 'schema', name: 'HR' }], kind: 'table', name: 'ORDERS' }

describe('CreateIndexDialog', () => {
  it('loads columns on open and preserves reordered columns, direction, and uniqueness', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(
      <QueryClientProvider
        client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
      >
        <CreateIndexDialog
          objectRef={objectRef}
          orgSlug="acme"
          workspaceId={1}
          connectionId={2}
          sessionId="session"
          pending={false}
          onClose={vi.fn()}
          onSubmit={onSubmit}
        />
      </QueryClientProvider>,
    )
    await screen.findByLabelText('Column 1')
    expect(queryFn).toHaveBeenCalled()
    fireEvent.change(screen.getByLabelText('Index name'), { target: { value: 'IX_ORDERS' } })
    await user.click(screen.getByLabelText('Column 1'))
    await user.click(screen.getByRole('option', { name: 'ID' }))
    await user.click(screen.getByRole('button', { name: 'Add column' }))
    await user.click(screen.getByLabelText('Column 2'))
    expect(screen.getByRole('option', { name: 'ID' })).toHaveAttribute('aria-disabled', 'true')
    await user.click(screen.getByRole('option', { name: 'LABEL' }))
    await user.click(screen.getAllByRole('checkbox', { name: 'Descending' })[1])
    await user.click(screen.getByRole('button', { name: 'Move column 2 up' }))
    await user.click(screen.getByRole('checkbox', { name: 'Unique index' }))
    await user.click(screen.getByRole('button', { name: 'Create index' }))
    expect(onSubmit).toHaveBeenCalledWith({
      operation: 'create_index',
      ref: objectRef,
      name: 'IX_ORDERS',
      unique: true,
      index_columns: [
        { name: 'LABEL', descending: true },
        { name: 'ID', descending: false },
      ],
    })
  })
})
