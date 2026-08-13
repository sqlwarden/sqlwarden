import { QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { Environment } from '#/lib/api/types'
import { createTestQueryClient } from '#/test/render'
import { server } from '#/test/server'
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

  it('discovers and persists the selected database and schema', async () => {
    const user = userEvent.setup()
    let createBody: Record<string, unknown> | undefined
    server.use(
      http.post('/api/v1/orgs/acme/workspaces/3/connections/test', () =>
        HttpResponse.json({
          ok: true,
          latency_ms: 12,
          scope_discovery: {
            current: [
              { kind: 'database', name: 'analytics' },
              { kind: 'schema', name: 'public' },
            ],
            scopes: [
              [{ kind: 'database', name: 'analytics' }],
              [{ kind: 'database', name: 'warehouse' }],
              [
                { kind: 'database', name: 'analytics' },
                { kind: 'schema', name: 'reporting' },
              ],
              [
                { kind: 'database', name: 'analytics' },
                { kind: 'schema', name: 'public' },
              ],
            ],
          },
        }),
      ),
      http.post('/api/v1/orgs/acme/workspaces/3/connections', async ({ request }) => {
        createBody = (await request.json()) as Record<string, unknown>
        return HttpResponse.json({ id: 9 }, { status: 201 })
      }),
    )
    renderDialog()
    await user.click(screen.getByRole('button', { name: /PostgreSQL/ }))
    await user.type(screen.getByPlaceholderText('My PostgreSQL'), 'Analytics')
    await user.type(screen.getByPlaceholderText('localhost'), 'db.example.test')
    await user.type(screen.getByPlaceholderText('postgres'), 'reader')

    await user.click(screen.getByRole('button', { name: 'Test Connection' }))
    expect(await screen.findByRole('combobox', { name: 'Default database' })).toHaveTextContent(
      'analytics',
    )
    expect(screen.queryByText('Database (optional)')).not.toBeInTheDocument()
    expect(screen.getByRole('combobox', { name: 'Default schema' })).toHaveTextContent('public')

    await user.click(screen.getByRole('combobox', { name: 'Default schema' }))
    await user.click(screen.getByRole('option', { name: 'Use database default' }))
    expect(screen.getByRole('combobox', { name: 'Default schema' })).toHaveTextContent(
      'Use database default',
    )
    expect(screen.queryByText('__sqlwarden_no_default_scope__')).not.toBeInTheDocument()

    await user.click(screen.getByRole('combobox', { name: 'Default database' }))
    await user.click(screen.getByRole('option', { name: 'No default database' }))
    expect(screen.getByRole('combobox', { name: 'Default database' })).toHaveTextContent(
      'No default database',
    )
    expect(screen.queryByText('__sqlwarden_no_default_scope__')).not.toBeInTheDocument()

    await user.click(screen.getByRole('combobox', { name: 'Default database' }))
    await user.click(await screen.findByRole('option', { name: 'analytics' }))
    await user.click(screen.getByRole('combobox', { name: 'Default schema' }))
    await user.click(await screen.findByRole('option', { name: 'reporting' }))
    await user.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() =>
      expect(createBody).toMatchObject({
        name: 'Analytics',
        driver: 'postgres',
        default_scope: [
          { kind: 'database', name: 'analytics' },
          { kind: 'schema', name: 'reporting' },
        ],
      }),
    )
    expect(createBody?.dsn).toContain('/analytics?')
  })

  it('toggles password field visibility', async () => {
    const user = userEvent.setup()
    renderDialog()
    await user.click(screen.getByRole('button', { name: /PostgreSQL/ }))

    expect(document.querySelector('input[type="password"]')).not.toBeNull()

    await user.click(screen.getByRole('button', { name: 'Show password' }))

    expect(document.querySelector('input[type="password"]')).toBeNull()
    expect(screen.getByRole('button', { name: 'Hide password' })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Hide password' }))

    expect(document.querySelector('input[type="password"]')).not.toBeNull()
  })
})
