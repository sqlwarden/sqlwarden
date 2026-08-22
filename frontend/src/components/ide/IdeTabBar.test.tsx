import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { Workspace } from '#/lib/api/types'
import { createTestQueryClient } from '#/test/render'
import type { GroupNode } from './ideLayout'
import { IdeTabBar, requiresCloseConfirmation } from './IdeTabBar'
import { createIdeStore, IdeStoreContext, useIde, type EditorTab } from './useIdeStore'

vi.mock('sonner', () => ({ toast: { error: vi.fn(), info: vi.fn() } }))

function tab(overrides: Partial<EditorTab> = {}): EditorTab {
  return {
    id: 'scratch:1',
    workspaceId: 3,
    title: 'Console 1',
    kind: 'scratch',
    content: '',
    ...overrides,
  }
}

describe('requiresCloseConfirmation', () => {
  it('allows clean and duplicated tab instances to close immediately', () => {
    expect(requiresCloseConfirmation(tab(), false, 1)).toBe(false)
    expect(requiresCloseConfirmation(tab({ content: 'select 1' }), false, 2)).toBe(false)
  })

  it('guards running queries, dirty files, and non-empty consoles', () => {
    expect(requiresCloseConfirmation(tab(), true, 1)).toBe(true)
    expect(
      requiresCloseConfirmation(tab({ kind: 'file', fileId: 7, isDirty: true }), false, 1),
    ).toBe(true)
    expect(requiresCloseConfirmation(tab({ content: 'select 1' }), false, 1)).toBe(true)
  })
})

const workspace: Workspace = {
  id: 3,
  org_id: 1,
  owner_type: 'org',
  owner_id: 1,
  name: 'Analytics',
  environment_count: 0,
  connection_count: 0,
  created_at: '',
  updated_at: '',
}

function TabBarHarness() {
  const group = useIde((state) => state.layout[3]) as GroupNode
  return <IdeTabBar orgSlug="acme" workspace={workspace} group={group} focused onFocus={vi.fn()} />
}

describe('IdeTabBar', () => {
  it('activates tabs, closes clean tabs, and confirms destructive closes', async () => {
    const store = createIdeStore('acme', 1, 'ephemeral')
    const queryClient = createTestQueryClient()
    const consoleTab = tab({ id: 'scratch:3:1', content: '' })
    const fileTab = tab({
      id: 'file:9',
      title: 'query.sql',
      kind: 'file',
      fileId: 9,
      isDirty: true,
    })
    store.setState({
      tabs: [consoleTab, fileTab],
      layout: {
        3: {
          type: 'group',
          id: 'main',
          tabIds: [consoleTab.id, fileTab.id],
          activeTabId: consoleTab.id,
        },
      },
      activeGroupId: { 3: 'main' },
    })
    const user = userEvent.setup()
    render(
      <QueryClientProvider client={queryClient}>
        <IdeStoreContext.Provider value={store}>
          <TabBarHarness />
        </IdeStoreContext.Provider>
      </QueryClientProvider>,
    )

    await user.click(screen.getByRole('tab', { name: /query.sql/ }))
    expect((store.getState().layout[3] as GroupNode).activeTabId).toBe(fileTab.id)

    await user.click(screen.getByRole('button', { name: 'Close Console 1' }))
    expect(store.getState().tabs.map((item) => item.id)).toEqual([fileTab.id])

    await user.click(screen.getByRole('button', { name: 'Close query.sql' }))
    expect(await screen.findByText('Close without saving?')).toBeInTheDocument()
    expect(store.getState().tabs).toHaveLength(1)
    await user.click(screen.getByRole('button', { name: 'Close anyway' }))
    expect(store.getState().tabs).toHaveLength(0)
  })

  it('opens a new console in the current workspace', async () => {
    const store = createIdeStore('acme', 1, 'ephemeral')
    const queryClient = createTestQueryClient()
    store.setState({
      layout: { 3: { type: 'group', id: 'main', tabIds: [], activeTabId: undefined } },
      activeGroupId: { 3: 'main' },
    })
    const user = userEvent.setup()
    render(
      <QueryClientProvider client={queryClient}>
        <IdeStoreContext.Provider value={store}>
          <TabBarHarness />
        </IdeStoreContext.Provider>
      </QueryClientProvider>,
    )

    await user.click(screen.getByRole('button', { name: 'New SQL console' }))
    expect(store.getState().tabs).toEqual([
      expect.objectContaining({ id: 'scratch:3:1', title: 'Console 1', workspaceId: 3 }),
    ])
  })

  it('guards closing the last tab on a connection with an open transaction', async () => {
    const store = createIdeStore('acme', 1, 'ephemeral')
    const queryClient = createTestQueryClient()
    const connTab = tab({
      id: 'scratch:3:2',
      title: 'Console 2',
      connectionId: 42,
    })
    store.setState({
      tabs: [connTab],
      layout: {
        3: { type: 'group', id: 'main', tabIds: [connTab.id], activeTabId: connTab.id },
      },
      activeGroupId: { 3: 'main' },
      sessions: { 42: 'session-42' },
      transactions: { 42: { mode: 'manual', open: true, pendingStatements: 2 } },
    })
    const user = userEvent.setup()
    render(
      <QueryClientProvider client={queryClient}>
        <IdeStoreContext.Provider value={store}>
          <TabBarHarness />
        </IdeStoreContext.Provider>
      </QueryClientProvider>,
    )

    await user.click(screen.getByRole('button', { name: 'Close Console 2' }))
    expect(await screen.findByText(/commit or roll back before closing/i)).toBeInTheDocument()
    expect(store.getState().tabs).toHaveLength(1)

    await user.click(screen.getByRole('button', { name: /cancel/i }))
    expect(store.getState().tabs).toHaveLength(1)
  })
})
