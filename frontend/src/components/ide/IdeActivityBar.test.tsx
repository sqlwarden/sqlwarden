import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Workspace } from '#/lib/api/types'
import { organizationRuntimeSettingsFixture } from '#/test/fixtures'
import { createTestQueryClient } from '#/test/render'
import { server } from '#/test/server'
import { IdeActivityBar } from './IdeActivityBar'
import { createIdeStore, IdeStoreContext } from './useIdeStore'

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

describe('IdeActivityBar', () => {
  beforeEach(() => {
    server.use(
      http.get('/api/v1/orgs/acme/runtime-settings', () =>
        HttpResponse.json(organizationRuntimeSettingsFixture()),
      ),
    )
  })

  it('toggles the active sidebar and expands it when switching activities', async () => {
    const store = createIdeStore('acme', 1, 'ephemeral')
    const user = userEvent.setup()
    const workspace = makeWorkspace(1, 'Analytics')
    render(
      <QueryClientProvider client={createTestQueryClient()}>
        <IdeStoreContext.Provider value={store}>
          <IdeActivityBar
            orgSlug="acme"
            workspaces={[workspace]}
            activeWorkspace={workspace}
            onSelectWorkspace={vi.fn()}
            session={undefined}
            canAccessOrgSettings={false}
          />
        </IdeStoreContext.Provider>
      </QueryClientProvider>,
    )

    expect(screen.getByRole('navigation', { name: 'Editor activities' })).toBeInTheDocument()
    const explorer = screen.getByRole('button', { name: 'Explorer' })
    expect(explorer).toHaveAttribute('aria-pressed', 'true')
    await user.click(explorer)
    expect(store.getState().sidebarCollapsed).toBe(true)

    await user.click(screen.getByRole('button', { name: 'Files' }))
    expect(store.getState().activeActivityId).toBe('files')
    expect(store.getState().sidebarCollapsed).toBe(false)
  })
})
