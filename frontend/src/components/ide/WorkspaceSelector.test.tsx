import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { Workspace } from '#/lib/api/types'
import { WorkspaceSelector } from './WorkspaceSelector'

function makeWorkspace(id: number, name: string): Workspace {
  return {
    id,
    org_id: 1,
    owner_type: 'org',
    owner_id: 1,
    name,
    environment_count: 0,
    connection_count: 0,
    created_at: '',
    updated_at: '',
  }
}

function renderSelector(props: Partial<React.ComponentProps<typeof WorkspaceSelector>> = {}) {
  const workspaces = props.workspaces ?? [
    makeWorkspace(1, 'Analytics'),
    makeWorkspace(2, 'Billing'),
  ]
  const rootRoute = createRootRoute({
    component: () => (
      <WorkspaceSelector
        workspaces={workspaces}
        activeWorkspace={workspaces[0]}
        onSelect={vi.fn()}
        {...props}
      />
    ),
  })
  const router = createRouter({
    routeTree: rootRoute,
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  render(<RouterProvider router={router} />)
  return { workspaces }
}

describe('WorkspaceSelector (compact, icon-only rail)', () => {
  it('exposes the active workspace name as the trigger label', async () => {
    renderSelector()
    expect(await screen.findByRole('combobox', { name: 'Analytics' })).toBeInTheDocument()
  })

  it('opens a popup listing every workspace, including the active one', async () => {
    const user = userEvent.setup()
    renderSelector()

    await user.click(await screen.findByRole('combobox', { name: 'Analytics' }))

    expect(await screen.findByRole('option', { name: /Billing/ })).toBeInTheDocument()
    expect(screen.getByRole('option', { name: /Analytics/ })).toBeInTheDocument()
  })

  it('calls onSelect with the chosen workspace id and closes the popup', async () => {
    const onSelect = vi.fn()
    const user = userEvent.setup()
    renderSelector({ onSelect })

    await user.click(await screen.findByRole('combobox', { name: 'Analytics' }))
    await user.click(await screen.findByRole('option', { name: /Billing/ }))

    expect(onSelect).toHaveBeenCalledWith(2)
    await waitFor(() => expect(screen.queryByRole('listbox')).not.toBeInTheDocument())
  })

  it('filters the workspace list by name', async () => {
    const user = userEvent.setup()
    renderSelector()

    await user.click(await screen.findByRole('combobox', { name: 'Analytics' }))
    await screen.findByRole('option', { name: /Billing/ })

    await user.type(screen.getByPlaceholderText('Find workspace...'), 'bill')

    expect(screen.getByRole('option', { name: /Billing/ })).toBeInTheDocument()
    expect(screen.queryByRole('option', { name: /Analytics/ })).not.toBeInTheDocument()
  })
})

describe('WorkspaceSelector (expanded rail)', () => {
  it('shows a visible name label alongside the icon', async () => {
    renderSelector({ expanded: true })
    expect(await screen.findByText('Analytics')).toBeInTheDocument()
  })

  it('keeps the trigger in place and opens a popup on click', async () => {
    const user = userEvent.setup()
    renderSelector({ expanded: true })

    const trigger = await screen.findByRole('combobox', { name: 'Analytics' })
    expect(screen.queryByRole('option', { name: /Billing/ })).not.toBeInTheDocument()

    await user.click(trigger)

    expect(await screen.findByRole('option', { name: /Billing/ })).toBeInTheDocument()
    expect(await screen.findByRole('combobox', { name: 'Analytics' })).toBe(trigger)
  })

  it('calls onSelect with the chosen workspace id and closes the popup again', async () => {
    const onSelect = vi.fn()
    const user = userEvent.setup()
    renderSelector({ expanded: true, onSelect })

    await user.click(await screen.findByRole('combobox', { name: 'Analytics' }))
    await user.click(await screen.findByRole('option', { name: /Billing/ }))

    expect(onSelect).toHaveBeenCalledWith(2)
    await waitFor(() => expect(screen.queryByRole('listbox')).not.toBeInTheDocument())
  })

  it('marks the active workspace as selected in the list', async () => {
    const user = userEvent.setup()
    renderSelector({ expanded: true })

    await user.click(await screen.findByRole('combobox', { name: 'Analytics' }))
    await screen.findByRole('option', { name: /Billing/ })

    expect(screen.getByRole('option', { name: /Analytics/ })).toHaveAttribute(
      'aria-selected',
      'true',
    )
  })
})
