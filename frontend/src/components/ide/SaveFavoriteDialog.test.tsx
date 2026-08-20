import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { SaveFavoriteDialog } from './SaveFavoriteDialog'

const createMock = vi.fn().mockResolvedValue(undefined)

vi.mock('./useFavoritesMutations', () => ({
  useFavoritesMutations: () => ({ create: createMock, update: vi.fn(), remove: vi.fn() }),
}))

describe('SaveFavoriteDialog', () => {
  beforeEach(() => {
    createMock.mockClear()
  })

  it('saves a favorite with the entered name', async () => {
    const onOpenChange = vi.fn()
    render(
      <SaveFavoriteDialog
        open
        onOpenChange={onOpenChange}
        orgSlug="acme"
        workspaceId={7}
        sqlText="select 1"
        connectionId={42}
      />,
    )

    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Top customers' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(createMock).toHaveBeenCalledWith({
        name: 'Top customers',
        sqlText: 'select 1',
        connectionId: 42,
      })
    })
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it('shows the SQL being favorited and calls onSaved after a successful save', async () => {
    const onOpenChange = vi.fn()
    const onSaved = vi.fn()
    render(
      <SaveFavoriteDialog
        open
        onOpenChange={onOpenChange}
        orgSlug="acme"
        workspaceId={7}
        sqlText="select * from widgets"
        connectionId={42}
        onSaved={onSaved}
      />,
    )

    expect(screen.getByText('select * from widgets')).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Widgets' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(onSaved).toHaveBeenCalled())
  })

  it('disables Save until a name is entered', () => {
    render(
      <SaveFavoriteDialog
        open
        onOpenChange={() => {}}
        orgSlug="acme"
        workspaceId={7}
        sqlText="select 1"
        connectionId={null}
      />,
    )

    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Top customers' } })
    expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled()
  })
})
