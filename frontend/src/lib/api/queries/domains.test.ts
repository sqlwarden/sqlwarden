import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from '#/lib/api/client'
import { setupStatusQueryOptions } from './auth'
import { instanceAccountsQueryOptions, instanceEditionQueryOptions } from './instance'
import { orgMembersQueryOptions } from './organization'
import { orgWorkspaceQueryOptions } from './workspace'
import { orgWorkspacePrivateFilesQueryOptions } from './files'
import { orgConnectionCatalogQueryOptions } from './database'

afterEach(() => vi.restoreAllMocks())

async function runQuery(options: { queryFn?: unknown }) {
  const queryFn = options.queryFn as ((context: unknown) => Promise<unknown>) | undefined
  if (!queryFn) throw new Error('Expected query function')
  return queryFn({})
}

describe('API query domains', () => {
  it('keeps setup status unauthenticated', async () => {
    const get = vi.spyOn(api, 'get').mockResolvedValue({ needs_setup: false })
    const options = setupStatusQueryOptions()
    await runQuery(options)
    expect(options.queryKey).toEqual(['setup-status'])
    expect(get).toHaveBeenCalledWith('/api/setup/status', { skipAuth: true })
  })

  it('passes list queries through instance and organization domains', async () => {
    const get = vi
      .spyOn(api, 'get')
      .mockResolvedValue({ items: [], page: 2, page_size: 20, total: 0 })
    const query = { page: 2, page_size: 20, q: 'alex' }
    await runQuery(instanceAccountsQueryOptions(query))
    await runQuery(orgMembersQueryOptions('acme', query))
    expect(get).toHaveBeenNthCalledWith(1, '/api/v1/instance/accounts', { query })
    expect(get).toHaveBeenNthCalledWith(2, '/api/v1/orgs/acme/members', { query })
  })

  it('refreshes edition capabilities instead of caching license state forever', () => {
    const options = instanceEditionQueryOptions()

    expect(options.staleTime).toBe(30_000)
    expect(options.refetchInterval).toBe(60_000)
    expect(options.refetchOnWindowFocus).toBe('always')
    expect(options.refetchOnReconnect).toBe('always')
  })

  it('builds workspace and file paths from explicit scope', async () => {
    const get = vi
      .spyOn(api, 'get')
      .mockResolvedValueOnce({ id: 4 })
      .mockResolvedValueOnce({ files: [] })
    await runQuery(orgWorkspaceQueryOptions('acme', 4))
    await runQuery(orgWorkspacePrivateFilesQueryOptions('acme', 4, 9))
    expect(get).toHaveBeenNthCalledWith(1, '/api/v1/orgs/acme/workspaces/4')
    expect(get).toHaveBeenNthCalledWith(2, '/api/v1/orgs/acme/workspaces/4/files/private', {
      query: { parent_id: 9 },
    })
  })

  it('sends the live database session for schema calls', async () => {
    const get = vi.spyOn(api, 'get').mockResolvedValue({ catalog: {} })
    await runQuery(orgConnectionCatalogQueryOptions('acme', 4, 7, 'session-1'))
    expect(get).toHaveBeenCalledWith(
      '/api/v1/orgs/acme/workspaces/4/connections/7/schema/catalog',
      { headers: { 'X-Warden-Session': 'session-1' } },
    )
  })
})
