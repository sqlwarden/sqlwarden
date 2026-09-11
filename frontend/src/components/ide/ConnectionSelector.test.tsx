import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { Connection, Environment } from '#/lib/api/types'
import { ConnectionSelector } from './ConnectionSelector'
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
    access_mode: 'open',
    show_system_schemas: false,
    created_at: '',
    updated_at: '',
  },
  {
    id: 8,
    workspace_id: 3,
    environment_id: 2,
    name: 'warehouse',
    driver: 'mysql',
    access_mode: 'open',
    show_system_schemas: false,
    created_at: '',
    updated_at: '',
  },
]

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
    await user.click(screen.getByRole('combobox', { name: 'Select connection' }))
    await user.type(screen.getByPlaceholderText('Search connections…'), 'warehouse')
    await user.click(await screen.findByRole('option', { name: /warehouse/ }))

    expect(onSelect).toHaveBeenCalledWith(connections[1])
    await waitFor(() => expect(screen.queryByRole('listbox')).not.toBeInTheDocument())
  })

  it('shows the connected connection name in the trigger', async () => {
    renderSelector({ active: connections[0] })
    expect(await screen.findByRole('combobox', { name: 'Select connection' })).toHaveTextContent(
      'app-db',
    )
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
    expect(screen.getByRole('combobox', { name: 'Select connection' })).toBeDisabled()
    unmount()

    renderSelector({ tabAvailable: false })
    expect(screen.getByRole('combobox', { name: 'Select connection' })).toBeDisabled()
  })
})
