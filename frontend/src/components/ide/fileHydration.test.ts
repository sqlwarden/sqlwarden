import { describe, expect, it, vi } from 'vitest'
import * as Y from 'yjs'
import { hydrateFileTab } from './fileHydration'
import type { EditorTab } from './useIdeStore'

const tab: EditorTab = {
  id: 'file:1',
  workspaceId: 2,
  title: 'query.sql',
  kind: 'file',
  fileId: 1,
  content: '',
}

describe('hydrateFileTab', () => {
  it('loads an empty document with the server-load origin and records its etag', () => {
    const doc = new Y.Doc()
    const origins: unknown[] = []
    doc.on('update', (_update, origin) => origins.push(origin))
    const updateTabEtag = vi.fn()

    hydrateFileTab(
      tab,
      { text: 'select 1', etag: 'etag-1' },
      { getOrCreate: () => doc },
      updateTabEtag,
    )

    expect(doc.getText('content').toString()).toBe('select 1')
    expect(origins).toContain('server-load')
    expect(updateTabEtag).toHaveBeenCalledWith('file:1', 'etag-1')
  })

  it('does not overwrite a document already hydrated by another window', () => {
    const doc = new Y.Doc()
    doc.getText('content').insert(0, 'peer content')
    hydrateFileTab(
      tab,
      { text: 'server content', etag: 'etag-1' },
      { getOrCreate: () => doc },
      vi.fn(),
    )
    expect(doc.getText('content').toString()).toBe('peer content')
  })

  it('does not overwrite dirty persisted state or clear an existing etag', () => {
    const getOrCreate = vi.fn(() => new Y.Doc())
    const updateTabEtag = vi.fn()
    hydrateFileTab(
      { ...tab, isDirty: true, ySnapshot: [1, 2, 3], etag: 'local-etag' },
      { text: 'server content', etag: 'server-etag' },
      { getOrCreate },
      updateTabEtag,
    )
    expect(getOrCreate).not.toHaveBeenCalled()
    expect(updateTabEtag).not.toHaveBeenCalled()
  })

  it('keeps an existing etag after loading content', () => {
    const updateTabEtag = vi.fn()
    hydrateFileTab(
      { ...tab, etag: 'existing-etag' },
      { text: 'server content', etag: 'server-etag' },
      { getOrCreate: () => new Y.Doc() },
      updateTabEtag,
    )
    expect(updateTabEtag).not.toHaveBeenCalled()
  })
})
