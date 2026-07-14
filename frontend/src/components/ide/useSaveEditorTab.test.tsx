import type { PropsWithChildren } from 'react'
import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import * as Y from 'yjs'
import { createIdeStore, IdeStoreContext, type EditorTab } from './useIdeStore'
import { YDocRegistryContext } from './useYDocRegistry'
import { useSaveEditorTab } from './useSaveEditorTab'

const mocks = vi.hoisted(() => ({
  toastError: vi.fn(),
  updateFile: vi.fn(),
}))

vi.mock('sonner', () => ({ toast: { error: mocks.toastError } }))
vi.mock('#/lib/api/files', () => ({ updatePrivateWorkspaceFileContent: mocks.updateFile }))

const fileTab: EditorTab = {
  id: 'file:9',
  workspaceId: 3,
  title: 'query.sql',
  kind: 'file',
  fileId: 9,
  content: 'stale content',
  etag: 'etag-1',
  isDirty: true,
}

describe('useSaveEditorTab', () => {
  beforeEach(() => {
    mocks.toastError.mockReset()
    mocks.updateFile.mockReset()
  })

  it('saves live Y.Doc content and updates the tab etag', async () => {
    const store = createIdeStore('acme', 1, 'ephemeral')
    store.setState({ tabs: [fileTab] })
    const doc = new Y.Doc()
    doc.getText('content').insert(0, 'select live')
    const registry = {
      get: vi.fn(() => doc),
      getOrCreate: vi.fn(),
      destroy: vi.fn(),
      disposeAll: vi.fn(),
    }
    mocks.updateFile.mockResolvedValue({ etag: 'etag-2' })
    function wrapper({ children }: PropsWithChildren) {
      return (
        <IdeStoreContext.Provider value={store}>
          <YDocRegistryContext.Provider value={registry}>{children}</YDocRegistryContext.Provider>
        </IdeStoreContext.Provider>
      )
    }
    const { result } = renderHook(() => useSaveEditorTab('acme', 3), { wrapper })

    await act(async () => result.current(fileTab))

    expect(mocks.updateFile).toHaveBeenCalledWith('acme', 3, 9, 'select live', 'etag-1')
    expect(store.getState().tabs[0]).toEqual(
      expect.objectContaining({ etag: 'etag-2', isDirty: false }),
    )
    doc.destroy()
  })

  it('returns undefined and distinguishes stale-file conflicts', async () => {
    const store = createIdeStore('acme', 1, 'ephemeral')
    const registry = {
      get: vi.fn(() => undefined),
      getOrCreate: vi.fn(),
      destroy: vi.fn(),
      disposeAll: vi.fn(),
    }
    mocks.updateFile.mockRejectedValue({ status: 412 })
    function wrapper({ children }: PropsWithChildren) {
      return (
        <IdeStoreContext.Provider value={store}>
          <YDocRegistryContext.Provider value={registry}>{children}</YDocRegistryContext.Provider>
        </IdeStoreContext.Provider>
      )
    }
    const { result } = renderHook(() => useSaveEditorTab('acme', 3), { wrapper })

    let saved: unknown
    await act(async () => {
      saved = await result.current(fileTab)
    })
    expect(saved).toBeUndefined()
    expect(mocks.toastError).toHaveBeenCalledWith('File changed externally. Reload before saving.')
  })
})
