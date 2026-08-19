import { act, renderHook } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { useToolbarQueryAction } from './useToolbarQueryAction'

vi.mock('./useEditorViewRegistry', () => ({ useEditorViewRegistry: () => new Map() }))
vi.mock('./useYDocRegistry', () => ({ useYDocRegistry: () => new Map() }))

const execute = vi.fn(async () => undefined)
vi.mock('./useQueryExecution', () => ({
  useQueryExecution: () => ({ cancel: vi.fn(), isRunning: false, run: execute }),
}))
vi.mock('./useIdeStore', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./useIdeStore')>()
  return {
    ...actual,
    useIde: (selector: (s: unknown) => unknown) =>
      selector({ maximizedPane: null, setMaximizedPane: vi.fn() }),
  }
})

describe('useToolbarQueryAction', () => {
  it('passes confirmUnsafe through to the underlying execute call', async () => {
    execute.mockClear()
    const { result } = renderHook(() =>
      useToolbarQueryAction({
        orgSlug: 'acme',
        workspace: { id: 1 } as never,
        activeTab: {
          id: 'tab-1',
          workspaceId: 1,
          kind: 'scratch',
          title: 'Console 1',
          content: 'DELETE FROM widgets',
        } as never,
        activeGroupId: 'grp-1',
        activeConnection: { id: 7, driver: 'sqlite' } as never,
        hasConnections: true,
      }),
    )

    await act(async () => {
      await result.current.run(true)
    })

    expect(execute).toHaveBeenCalledWith('DELETE FROM widgets', true)
  })
})
