import { render, screen } from '@testing-library/react'
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

describe('WorkspaceSelector', () => {
  const workspaces = [makeWorkspace(1, 'Analytics'), makeWorkspace(2, 'Billing')]

  it('shows the active workspace name as the trigger label', () => {
    render(
      <WorkspaceSelector
        workspaces={workspaces}
        activeWorkspace={workspaces[0]}
        onSelect={vi.fn()}
      />,
    )
    expect(screen.getByRole('button', { name: 'Analytics' })).toBeInTheDocument()
  })

  it('calls onSelect with the chosen workspace id', async () => {
    const onSelect = vi.fn()
    const user = userEvent.setup()
    render(
      <WorkspaceSelector
        workspaces={workspaces}
        activeWorkspace={workspaces[0]}
        onSelect={onSelect}
      />,
    )

    await user.click(screen.getByRole('button', { name: 'Analytics' }))
    await user.click(await screen.findByRole('menuitem', { name: 'Billing' }))

    expect(onSelect).toHaveBeenCalledWith(2)
  })

  it('shows a visible name label alongside the icon when expanded', () => {
    render(
      <WorkspaceSelector
        workspaces={workspaces}
        activeWorkspace={workspaces[0]}
        onSelect={vi.fn()}
        expanded
      />,
    )
    expect(screen.getByText('Analytics')).toBeInTheDocument()
  })
})
