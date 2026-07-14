import { describe, expect, it, vi } from 'vitest'
import { saveEditorTab, type SaveEditorTabDependencies } from './filePersistence'
import type { EditorTab } from './useIdeStore'

const fileTab: EditorTab = {
  id: 'file:12',
  workspaceId: 3,
  kind: 'file',
  title: 'query.sql',
  fileId: 12,
  etag: 'old-etag',
  content: 'stale snapshot',
}

function dependencies(): SaveEditorTabDependencies {
  return {
    readContent: vi.fn(() => 'current Y.Doc content'),
    updateFile: vi.fn(async () => ({ etag: 'new-etag' })),
    updateTabEtag: vi.fn(),
  }
}

describe('saveEditorTab', () => {
  it('saves current editor content with the existing etag', async () => {
    const deps = dependencies()
    await expect(saveEditorTab(fileTab, deps)).resolves.toEqual({ kind: 'saved' })
    expect(deps.updateFile).toHaveBeenCalledWith(12, 'current Y.Doc content', 'old-etag')
    expect(deps.updateTabEtag).toHaveBeenCalledWith('file:12', 'new-etag')
  })

  it('returns a current snapshot for Save As when the tab is not an existing file', async () => {
    const deps = dependencies()
    const scratch: EditorTab = {
      id: 'scratch:3:1',
      workspaceId: 3,
      kind: 'scratch',
      title: 'Console 1',
      content: 'stale snapshot',
    }
    const result = await saveEditorTab(scratch, deps)
    expect(result).toEqual({
      kind: 'save-as',
      tab: { ...scratch, content: 'current Y.Doc content' },
    })
    expect(deps.updateFile).not.toHaveBeenCalled()
  })

  it('does not mark a tab clean when the update fails', async () => {
    const deps = dependencies()
    vi.mocked(deps.updateFile).mockRejectedValue(new Error('conflict'))
    await expect(saveEditorTab(fileTab, deps)).rejects.toThrow('conflict')
    expect(deps.updateTabEtag).not.toHaveBeenCalled()
  })
})
