import type { PropsWithChildren } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { Workspace } from '#/lib/api/types'
import type { LayoutNode } from './ideLayout'
import { EditorLayout } from './EditorLayout'
import { createIdeStore, IdeStoreContext } from './useIdeStore'

vi.mock('./EditorGroup', () => ({
  EditorGroup: ({
    group,
    focused,
    showFocus,
  }: {
    group: { id: string }
    focused: boolean
    showFocus?: boolean
  }) => (
    <div data-testid={`group-${group.id}`} data-focused={focused} data-show-focus={showFocus}>
      {group.id}
    </div>
  ),
}))

vi.mock('#/components/ui/resizable', () => ({
  ResizablePanelGroup: ({
    children,
    orientation,
    defaultLayout,
    onLayoutChanged,
    id,
  }: PropsWithChildren<{
    orientation: string
    defaultLayout: Record<string, number>
    onLayoutChanged: (layout: Record<string, number>) => void
    id: string
  }>) => (
    <div
      data-testid={`split-${id}`}
      data-orientation={orientation}
      data-layout={JSON.stringify(defaultLayout)}
    >
      {children}
      <button type="button" onClick={() => onLayoutChanged({ left: 40, right: 60 })}>
        resize {id}
      </button>
    </div>
  ),
  ResizablePanel: ({ children }: PropsWithChildren) => <div>{children}</div>,
  ResizableHandle: () => <span data-testid="resize-handle" />,
}))

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

const layout: LayoutNode = {
  type: 'split',
  id: 'root',
  orientation: 'row',
  sizes: [30, 70],
  children: [
    { type: 'group', id: 'left', tabIds: ['one'], activeTabId: 'one' },
    { type: 'group', id: 'right', tabIds: ['two'], activeTabId: 'two' },
  ],
}

describe('EditorLayout', () => {
  it('renders persisted split geometry and marks the focused pane', () => {
    const store = createIdeStore('acme', 1, 'ephemeral')
    store.setState({ layout: { 3: layout }, activeGroupId: { 3: 'right' } })

    render(
      <IdeStoreContext.Provider value={store}>
        <EditorLayout orgSlug="acme" workspace={workspace} node={layout} />
      </IdeStoreContext.Provider>,
    )

    expect(screen.getByTestId('split-root')).toHaveAttribute('data-orientation', 'horizontal')
    expect(screen.getByTestId('split-root')).toHaveAttribute(
      'data-layout',
      JSON.stringify({ left: 30, right: 70 }),
    )
    expect(screen.getByTestId('group-left')).toHaveAttribute('data-focused', 'false')
    expect(screen.getByTestId('group-right')).toHaveAttribute('data-focused', 'true')
    expect(screen.getAllByTestId('resize-handle')).toHaveLength(1)
  })

  it('persists the final panel sizes in the matching split node', async () => {
    const store = createIdeStore('acme', 1, 'ephemeral')
    store.setState({ layout: { 3: layout }, activeGroupId: { 3: 'left' } })
    const user = userEvent.setup()
    render(
      <IdeStoreContext.Provider value={store}>
        <EditorLayout orgSlug="acme" workspace={workspace} node={layout} />
      </IdeStoreContext.Provider>,
    )

    await user.click(screen.getByRole('button', { name: 'resize root' }))
    expect(store.getState().layout[3]).toEqual(expect.objectContaining({ sizes: [40, 60] }))
  })
})
