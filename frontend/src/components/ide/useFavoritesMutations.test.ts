import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { renderHook } from '@testing-library/react'
import { createElement, type ReactNode } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { queryKeys } from '#/lib/api/query-keys'
import * as queryFavoritesApi from '#/lib/api/queries/query-favorites'
import * as localStore from './localQueryStore'
import { useFavoritesMutations } from './useFavoritesMutations'

function wrapper(client: QueryClient) {
  return ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client }, children)
}

describe('useFavoritesMutations', () => {
  it('creates a favorite on the backend when effective mode is backend', async () => {
    const client = new QueryClient()
    client.setQueryData(queryKeys.orgRuntimeSettings('acme'), {
      effective: { query_favorites_mode: 'backend' },
    })
    const createSpy = vi
      .spyOn(queryFavoritesApi, 'createQueryFavorite')
      .mockResolvedValue(undefined as never)

    const { result } = renderHook(() => useFavoritesMutations('acme', 7), {
      wrapper: wrapper(client),
    })
    await result.current.create({ name: 'Top customers', sqlText: 'select 1', connectionId: 42 })

    expect(createSpy).toHaveBeenCalledWith('acme', 7, {
      name: 'Top customers',
      sql_text: 'select 1',
      connection_id: 42,
    })
  })

  it('creates a favorite locally when effective mode is local', async () => {
    const client = new QueryClient()
    client.setQueryData(queryKeys.orgRuntimeSettings('acme'), {
      effective: { query_favorites_mode: 'local' },
    })
    const localSpy = vi.spyOn(localStore, 'addLocalFavorite').mockResolvedValue(undefined as never)

    const { result } = renderHook(() => useFavoritesMutations('acme', 7), {
      wrapper: wrapper(client),
    })
    await result.current.create({ name: 'Top customers', sqlText: 'select 1', connectionId: null })

    expect(localSpy).toHaveBeenCalledWith({
      workspaceId: 7,
      connectionId: null,
      name: 'Top customers',
      sqlText: 'select 1',
    })
  })

  it('does nothing when effective mode is off', async () => {
    const client = new QueryClient()
    client.setQueryData(queryKeys.orgRuntimeSettings('acme'), {
      effective: { query_favorites_mode: 'off' },
    })
    const createSpy = vi
      .spyOn(queryFavoritesApi, 'createQueryFavorite')
      .mockResolvedValue(undefined as never)
    const localSpy = vi.spyOn(localStore, 'addLocalFavorite').mockResolvedValue(undefined as never)

    const { result } = renderHook(() => useFavoritesMutations('acme', 7), {
      wrapper: wrapper(client),
    })
    await result.current.create({ name: 'Top customers', sqlText: 'select 1', connectionId: null })

    expect(createSpy).not.toHaveBeenCalled()
    expect(localSpy).not.toHaveBeenCalled()
  })

  it('deletes a favorite on the backend and invalidates the favorites list', async () => {
    const client = new QueryClient()
    client.setQueryData(queryKeys.orgRuntimeSettings('acme'), {
      effective: { query_favorites_mode: 'backend' },
    })
    const deleteSpy = vi
      .spyOn(queryFavoritesApi, 'deleteQueryFavorite')
      .mockResolvedValue(undefined as never)
    const invalidateSpy = vi.spyOn(client, 'invalidateQueries')

    const { result } = renderHook(() => useFavoritesMutations('acme', 7), {
      wrapper: wrapper(client),
    })
    await result.current.remove(3)

    expect(deleteSpy).toHaveBeenCalledWith('acme', 7, 3)
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: queryKeys.orgWorkspaceQueryFavoritesScope('acme', 7),
    })
  })
})
