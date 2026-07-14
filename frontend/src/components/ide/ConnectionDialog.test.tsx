import { QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { Environment } from '#/lib/api/types'
import { createTestQueryClient } from '#/test/render'
import { drivers } from './connection-drivers'
import { ConnectionDialog } from './ConnectionDialog'

vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))

const environment: Environment = {
  id: 4,
  workspace_id: 3,
  name: 'Development',
  created_at: '',
  updated_at: '',
}

function renderDialog(overrides: { lockedEnvironmentId?: number } = {}) {
  const onOpenChange = vi.fn()
  render(
    <QueryClientProvider client={createTestQueryClient()}>
      <ConnectionDialog
        open={true}
        onOpenChange={onOpenChange}
        orgSlug="acme"
        workspaceId={3}
        environments={[environment]}
        lockedEnvironmentId={overrides.lockedEnvironmentId}
      />
    </QueryClientProvider>,
  )
  return { onOpenChange }
}

describe('ConnectionDialog', () => {
  it('filters the build-time driver registry without a fallback option', async () => {
    const user = userEvent.setup()
    renderDialog()

    expect(screen.getByRole('heading', { name: 'Choose a database' })).toBeInTheDocument()
    for (const driver of drivers) {
      expect(screen.getByRole('button', { name: new RegExp(driver.label) })).toBeInTheDocument()
    }

    await user.type(screen.getByPlaceholderText('Search databases…'), 'not-a-real-engine')
    expect(screen.getByText(/No databases match/)).toBeInTheDocument()
  })

  it('renders registry-driven fields and can return to driver selection', () => {
    renderDialog({ lockedEnvironmentId: 4 })
    fireEvent.click(screen.getByRole('button', { name: new RegExp(drivers[0].label) }))

    expect(screen.getByRole('heading', { name: 'New Connection' })).toBeInTheDocument()
    for (const field of drivers[0].fields) {
      expect(screen.getByText(field.label)).toBeInTheDocument()
    }
    expect(
      screen.getAllByRole('combobox').some((combobox) => combobox.hasAttribute('disabled')),
    ).toBe(true)

    fireEvent.click(screen.getByRole('button', { name: 'Change' }))
    expect(screen.getByRole('heading', { name: 'Choose a database' })).toBeInTheDocument()
  })
})
