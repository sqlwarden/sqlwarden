import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { Connection, Environment } from '#/lib/api/types'
import { ConnectionSelector, groupConnections } from './ConnectionSelector'
import { createIdeStore, IdeStoreContext } from './useIdeStore'

const environments: Environment[] = [
  { id: 1, workspace_id: 3, name: 'Development', created_at: '', updated_at: '' },
  { id: 2, workspace_id: 3, name: 'Production', created_at: '', updated_at: '' },
]
const connections: Connection[] = [
  {
    id: 7,
    workspace_id: 3,
    environment_id: 1,
    name: 'app-db',
    driver: 'postgres',
    created_at: '',
    updated_at: '',
  },
  {
    id: 8,
    workspace_id: 3,
    environment_id: 2,
    name: 'warehouse',
    driver: 'mysql',
    created_at: '',
    updated_at: '',
  },
]

describe('groupConnections', () => {
  it('groups by environment and searches both environment and connection names', () => {
    expect(groupConnections(environments, connections, '')).toHaveLength(2)
    expect(groupConnections(environments, connections, 'prod')).toEqual([
      { environment: environments[1], connections: [connections[1]] },
    ])
    expect(groupConnections(environments, connections, 'app')).toEqual([
      { environment: environments[0], connections: [connections[0]] },
    ])
    expect(groupConnections(environments, connections, 'missing')).toEqual([])
  })
})

describe('ConnectionSelector', () => {
  function renderSelector(
    options: { active?: Connection; loading?: boolean; tabAvailable?: boolean } = {},
  ) {
    const store = createIdeStore('acme', 1, 'ephemeral')
    store.getState().setSession(7, 'session-7')
    const onSelect = vi.fn()
    render(
      <IdeStoreContext.Provider value={store}>
        <ConnectionSelector
          activeConnection={options.active}
          activeConnectionId={options.active?.id}
          connections={connections}
          environments={environments}
          isLoading={options.loading ?? false}
          tabAvailable={options.tabAvailable ?? true}
          onSelect={onSelect}
        />
      </IdeStoreContext.Provider>,
    )
    return { onSelect }
  }

  it('selects a filtered connection and closes the picker', async () => {
    const user = userEvent.setup()
    const { onSelect } = renderSelector()
    await user.click(screen.getByRole('button', { name: /Select connection/ }))
    await user.type(screen.getByPlaceholderText('Search connections…'), 'warehouse')
    fireEvent.click(screen.getByRole('button', { name: /warehouse/ }))

    expect(onSelect).toHaveBeenCalledWith(connections[1])
    await waitFor(() =>
      expect(screen.queryByPlaceholderText('Search connections…')).not.toBeInTheDocument(),
    )
  })

  it('shows connected state and disables selection without an editor tab', () => {
    const view = renderSelector({ active: connections[0] })
    expect(screen.getByRole('button', { name: /app-db/ })).toBeEnabled()
    view.onSelect.mockClear()
  })

  it('disables the trigger while connections load or no tab is active', () => {
    const { unmount } = render(
      <IdeStoreContext.Provider value={createIdeStore('acme', 1, 'ephemeral')}>
        <ConnectionSelector
          connections={connections}
          environments={environments}
          isLoading
          tabAvailable
          onSelect={vi.fn()}
        />
      </IdeStoreContext.Provider>,
    )
    expect(screen.getByRole('button', { name: /Loading connections/ })).toBeDisabled()
    unmount()

    renderSelector({ tabAvailable: false })
    expect(screen.getByRole('button', { name: /Select connection/ })).toBeDisabled()
  })
})
