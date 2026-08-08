import type { PropsWithChildren } from 'react'
import { QueryClientProvider } from '@tanstack/react-query'
import { renderHook, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it, vi } from 'vitest'
import * as Y from 'yjs'
import { createTestQueryClient } from '#/test/render'
import { server } from '#/test/server'
import { useFileContent } from './useFileContent'
import { YDocRegistryContext, type YDocRegistry } from './useYDocRegistry'
import type { EditorTab } from './useIdeStore'

const tab: EditorTab = {
  id: 'file:11',
  workspaceId: 3,
  title: 'query.sql',
  kind: 'file',
  fileId: 11,
  content: '',
}

describe('useFileContent', () => {
  function setup() {
    const doc = new Y.Doc()
    const registry: YDocRegistry = {
      getOrCreate: () => doc,
      get: () => doc,
      destroy: vi.fn(),
      disposeAll: vi.fn(),
    }
    const queryClient = createTestQueryClient()
    function wrapper({ children }: PropsWithChildren) {
      return (
        <QueryClientProvider client={queryClient}>
          <YDocRegistryContext.Provider value={registry}>{children}</YDocRegistryContext.Provider>
        </QueryClientProvider>
      )
    }
    return { doc, wrapper }
  }

  it('hydrates server content and records the response ETag', async () => {
    server.use(
      http.get(
        '/api/v1/orgs/acme/workspaces/3/files/private/11/content',
        () => new HttpResponse('select 42', { headers: { ETag: '"etag-11"' } }),
      ),
    )
    const updateTabEtag = vi.fn()
    const { doc, wrapper } = setup()
    const { result } = renderHook(
      () =>
        useFileContent({
          orgSlug: 'acme',
          workspaceId: 3,
          tab,
          updateTabEtag,
        }),
      { wrapper },
    )

    expect(result.current.isLoading).toBe(true)
    await waitFor(() => expect(doc.getText('content').toString()).toBe('select 42'))
    expect(updateTabEtag).toHaveBeenCalledWith('file:11', 'etag-11')
    expect(result.current.isError).toBe(false)
  })

  it('does not fetch for non-file tabs', async () => {
    const request = vi.fn()
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/files/private/:fileId/content', () => {
        request()
        return new HttpResponse('unexpected')
      }),
    )
    const { wrapper } = setup()
    const { result } = renderHook(
      () =>
        useFileContent({
          orgSlug: 'acme',
          workspaceId: 3,
          tab: { ...tab, kind: 'scratch', fileId: undefined },
          updateTabEtag: vi.fn(),
        }),
      { wrapper },
    )

    expect(result.current.isLoading).toBe(false)
    await Promise.resolve()
    expect(request).not.toHaveBeenCalled()
  })

  it('does not fetch when content loading is disabled', async () => {
    const request = vi.fn()
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/files/private/11/content', () => {
        request()
        return new HttpResponse('unexpected')
      }),
    )
    const { wrapper } = setup()
    const { result } = renderHook(
      () =>
        useFileContent({
          orgSlug: 'acme',
          workspaceId: 3,
          tab,
          updateTabEtag: vi.fn(),
          enabled: false,
        }),
      { wrapper },
    )

    expect(result.current.isLoading).toBe(false)
    await Promise.resolve()
    expect(request).not.toHaveBeenCalled()
  })

  it('exposes failed loads and retries them on demand', async () => {
    let attempts = 0
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/files/private/11/content', () => {
        attempts += 1
        return attempts === 1
          ? HttpResponse.text('Unavailable', { status: 503 })
          : new HttpResponse('select 7', { headers: { ETag: 'etag-7' } })
      }),
    )
    const { doc, wrapper } = setup()
    const { result } = renderHook(
      () =>
        useFileContent({
          orgSlug: 'acme',
          workspaceId: 3,
          tab,
          updateTabEtag: vi.fn(),
        }),
      { wrapper },
    )

    await waitFor(() => expect(result.current.isError).toBe(true))
    result.current.retry()
    await waitFor(() => expect(doc.getText('content').toString()).toBe('select 7'))
    expect(attempts).toBe(2)
  })
})
