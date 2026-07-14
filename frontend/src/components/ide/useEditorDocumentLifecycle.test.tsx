import { act, renderHook } from '@testing-library/react'
import { QueryClient } from '@tanstack/react-query'
import * as Y from 'yjs'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { EditorTab } from './useIdeStore'
import type { YDocRegistry } from './useYDocRegistry'
import {
  useClosedFileCacheCleanup,
  useEditorDocumentLifecycle,
  useEditorSaveShortcut,
  useEditorSnapshotPersistence,
} from './useEditorDocumentLifecycle'

function tab(overrides: Partial<EditorTab> = {}): EditorTab {
  return {
    id: 'tab-1',
    workspaceId: 3,
    title: 'Console',
    kind: 'scratch',
    content: 'select 1',
    ...overrides,
  }
}

function yState(content: string) {
  const doc = new Y.Doc()
  doc.getText('content').insert(0, content)
  const state = Array.from(Y.encodeStateAsUpdate(doc))
  doc.destroy()
  return state
}

function fakeRegistry() {
  const docs = new Map<string, Y.Doc>()
  const destroy = vi.fn((id: string) => {
    docs.get(id)?.destroy()
    docs.delete(id)
  })
  const registry: YDocRegistry = {
    getOrCreate(id, initialContent) {
      let doc = docs.get(id)
      if (!doc) {
        doc = new Y.Doc()
        if (initialContent) {
          doc.getText('content').insert(0, initialContent)
        }
        docs.set(id, doc)
      }
      return doc
    },
    get: (id) => docs.get(id),
    destroy,
    disposeAll() {
      for (const doc of docs.values()) doc.destroy()
      docs.clear()
    },
  }
  return { destroy, docs, registry }
}

afterEach(() => vi.useRealTimers())

describe('editor document lifecycle', () => {
  it('uses snapshot state before creation state and plain content', () => {
    const { registry } = fakeRegistry()
    const current = tab({
      content: 'plain content',
      yState: yState('creation state'),
      ySnapshot: yState('latest snapshot'),
    })

    renderHook(() => useEditorDocumentLifecycle([current], registry))

    expect(registry.get('tab-1')?.getText('content').toString()).toBe('latest snapshot')
  })

  it('does not seed file documents from stale plain content and destroys closed tabs', () => {
    const { destroy, registry } = fakeRegistry()
    const file = tab({ id: 'file:9', kind: 'file', fileId: 9, content: 'stale' })
    const { rerender } = renderHook(({ tabs }) => useEditorDocumentLifecycle(tabs, registry), {
      initialProps: { tabs: [file] },
    })

    expect(registry.get(file.id)?.getText('content').toString()).toBe('')
    rerender({ tabs: [] })
    expect(destroy).toHaveBeenCalledWith(file.id)
  })

  it('persists user and broadcast updates after the debounce but ignores initialization', () => {
    vi.useFakeTimers()
    const { registry } = fakeRegistry()
    const updateTabContent = vi.fn()
    const current = tab({ content: '' })
    renderHook(() => {
      useEditorDocumentLifecycle([current], registry)
      useEditorSnapshotPersistence([current], registry, updateTabContent)
    })
    const doc = registry.get(current.id)!

    act(() => doc.transact(() => doc.getText('content').insert(0, 'server'), 'server-load'))
    act(() => vi.advanceTimersByTime(400))
    expect(updateTabContent).not.toHaveBeenCalled()

    act(() => doc.transact(() => doc.getText('content').insert(6, ' edit'), 'broadcast'))
    act(() => vi.advanceTimersByTime(399))
    expect(updateTabContent).not.toHaveBeenCalled()
    act(() => vi.advanceTimersByTime(1))
    expect(updateTabContent).toHaveBeenCalledWith(current.id, 'server edit', expect.any(Array))
  })

  it('cancels pending snapshot writes on unmount', () => {
    vi.useFakeTimers()
    const { registry } = fakeRegistry()
    const updateTabContent = vi.fn()
    const current = tab({ content: '' })
    const { unmount } = renderHook(() => {
      useEditorDocumentLifecycle([current], registry)
      useEditorSnapshotPersistence([current], registry, updateTabContent)
    })

    act(() => registry.get(current.id)!.getText('content').insert(0, 'pending'))
    unmount()
    act(() => vi.advanceTimersByTime(400))
    expect(updateTabContent).not.toHaveBeenCalled()
  })
})

describe('editor cleanup and shortcuts', () => {
  it('removes a closed file from its own workspace cache only', () => {
    const queryClient = new QueryClient()
    const removeQueries = vi.spyOn(queryClient, 'removeQueries')
    const file = tab({ id: 'file:4', workspaceId: 12, kind: 'file', fileId: 4 })
    const { rerender } = renderHook(
      ({ tabs }) => useClosedFileCacheCleanup(tabs, 'acme', queryClient),
      { initialProps: { tabs: [file] } },
    )

    rerender({ tabs: [] })
    expect(removeQueries).toHaveBeenCalledWith({
      queryKey: ['file-content', 'acme', 12, 4],
    })
  })

  it('saves only an existing file tab and removes the shortcut on unmount', () => {
    const save = vi.fn().mockResolvedValue(undefined)
    const file = tab({ kind: 'file', fileId: 4, etag: 'v1' })
    const { unmount } = renderHook(() => useEditorSaveShortcut(file, save))

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 's', ctrlKey: true }))
    expect(save).toHaveBeenCalledWith(file)
    unmount()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 's', ctrlKey: true }))
    expect(save).toHaveBeenCalledOnce()
  })

  it('does not save scratch or unsaved file tabs', () => {
    const save = vi.fn().mockResolvedValue(undefined)
    renderHook(() => useEditorSaveShortcut(tab(), save))
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 's', metaKey: true }))
    expect(save).not.toHaveBeenCalled()
  })
})
