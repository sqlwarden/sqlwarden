import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { Connection, Environment } from '#/lib/api/types'
import { ALL_CONNECTIONS, ConnectionCombobox } from './ConnectionCombobox'

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

describe('ConnectionCombobox', () => {
  it('groups connections by environment and selects a filtered connection', async () => {
    const user = userEvent.setup()
    const onSelectConnection = vi.fn()
    render(
      <ConnectionCombobox
        connections={connections}
        environments={environments}
        sessions={{}}
        isLoading={false}
        placeholder="Select connection…"
        onSelectConnection={onSelectConnection}
      />,
    )

    await user.click(screen.getByRole('combobox', { name: 'Select connection…' }))
    expect(await screen.findByText('Development')).toBeInTheDocument()
    expect(screen.getByText('Production')).toBeInTheDocument()

    await user.type(screen.getByPlaceholderText('Search connections…'), 'warehouse')
    expect(await screen.findByRole('option', { name: /warehouse/ })).toBeInTheDocument()
    expect(screen.queryByRole('option', { name: /app-db/ })).not.toBeInTheDocument()

    await user.click(screen.getByRole('option', { name: /warehouse/ }))
    expect(onSelectConnection).toHaveBeenCalledWith(connections[1])
    await waitFor(() => expect(screen.queryByRole('listbox')).not.toBeInTheDocument())
  })

  it('matches connections by environment name search too', async () => {
    const user = userEvent.setup()
    render(
      <ConnectionCombobox
        connections={connections}
        environments={environments}
        sessions={{}}
        isLoading={false}
        placeholder="Select connection…"
        onSelectConnection={vi.fn()}
      />,
    )

    await user.click(screen.getByRole('combobox', { name: 'Select connection…' }))
    await user.type(await screen.findByPlaceholderText('Search connections…'), 'prod')

    expect(await screen.findByRole('option', { name: /warehouse/ })).toBeInTheDocument()
    expect(screen.queryByRole('option', { name: /app-db/ })).not.toBeInTheDocument()
  })

  it('pins an "All connections" option above the groups when includeAllOption is set', async () => {
    const user = userEvent.setup()
    const onSelectAll = vi.fn()
    render(
      <ConnectionCombobox
        connections={connections}
        environments={environments}
        sessions={{}}
        isLoading={false}
        includeAllOption
        placeholder="All connections"
        onSelectConnection={vi.fn()}
        onSelectAll={onSelectAll}
      />,
    )

    await user.click(screen.getByRole('combobox', { name: 'All connections' }))
    await user.click(await screen.findByRole('option', { name: 'All connections' }))

    expect(onSelectAll).toHaveBeenCalledOnce()
  })

  it('shows the driver badge and live-session dot for the selected connection', async () => {
    render(
      <ConnectionCombobox
        connections={connections}
        environments={environments}
        sessions={{ 7: 'session-7' }}
        isLoading={false}
        value={7}
        placeholder="Select connection…"
        onSelectConnection={vi.fn()}
      />,
    )

    expect(await screen.findByRole('combobox', { name: 'Select connection…' })).toHaveTextContent(
      'app-db',
    )
  })

  it('disables the trigger while loading or when there are no connections', () => {
    const { rerender } = render(
      <ConnectionCombobox
        connections={[]}
        environments={environments}
        sessions={{}}
        isLoading
        placeholder="Loading connections…"
        onSelectConnection={vi.fn()}
      />,
    )
    expect(screen.getByRole('combobox', { name: 'Loading connections…' })).toBeDisabled()

    rerender(
      <ConnectionCombobox
        connections={[]}
        environments={environments}
        sessions={{}}
        isLoading={false}
        placeholder="No connections"
        onSelectConnection={vi.fn()}
      />,
    )
    expect(screen.getByRole('combobox', { name: 'No connections' })).toBeDisabled()
  })

  it('exposes the ALL_CONNECTIONS sentinel', () => {
    expect(ALL_CONNECTIONS).toBe('all')
  })
})
