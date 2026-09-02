import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it, vi } from 'vitest'
import type { ObjectDetail, ObjectRef } from '#/lib/api/types'
import { createTestQueryClient } from '#/test/render'
import { server } from '#/test/server'
import type { ObjectViewModel } from '../registry'
import { DdlSection } from './DdlSection'

vi.mock('../ReadOnlySqlView', () => ({
  ReadOnlySqlView: ({ value }: { value: string }) => <pre data-testid="sql">{value}</pre>,
}))

const ref: ObjectRef = {
  scope: [{ kind: 'schema', name: 'public' }],
  kind: 'table',
  name: 'orders',
}

function vmFor(detail: ObjectDetail): ObjectViewModel {
  return {
    detail,
    dialect: {} as ObjectViewModel['dialect'],
    driver: 'oracle',
    orgSlug: 'acme',
    workspaceId: 3,
    connectionId: 7,
    sessionId: 'session-7',
  }
}

const definitionURL = '/api/v1/orgs/acme/workspaces/3/connections/7/schema/object/definition'

function renderSection(vm: ObjectViewModel) {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <DdlSection vm={vm} />
    </QueryClientProvider>,
  )
}

describe('DdlSection', () => {
  it('renders an inline DDL descriptor without a network request', () => {
    let called = false
    server.use(
      http.get(definitionURL, () => {
        called = true
        return HttpResponse.json({ descriptor: null })
      }),
    )
    renderSection(
      vmFor({
        ref,
        descriptors: [
          {
            kind: 'source',
            title: 'DDL',
            source: { language: 'sql', body: 'CREATE TABLE inline_orders' },
          },
        ],
      }),
    )
    expect(screen.getByTestId('sql')).toHaveTextContent('CREATE TABLE inline_orders')
    expect(called).toBe(false)
  })

  it('lazily fetches the definition when no inline descriptor is present', async () => {
    server.use(
      http.get(definitionURL, ({ request }) => {
        const url = new URL(request.url)
        expect(url.searchParams.get('kind')).toBe('table')
        expect(url.searchParams.get('name')).toBe('orders')
        return HttpResponse.json({
          descriptor: {
            kind: 'source',
            title: 'DDL',
            source: { language: 'sql', body: 'CREATE TABLE lazy_orders' },
          },
        })
      }),
    )
    renderSection(vmFor({ ref, descriptors: [] }))
    expect(await screen.findByTestId('sql')).toHaveTextContent('CREATE TABLE lazy_orders')
  })

  it('shows a fallback when the lazy definition is unavailable', async () => {
    server.use(http.get(definitionURL, () => HttpResponse.json({ descriptor: null })))
    renderSection(vmFor({ ref, descriptors: [] }))
    await waitFor(() => expect(screen.getByText('No definition available.')).toBeInTheDocument())
  })

  it('shows an error fallback when the lazy fetch fails', async () => {
    server.use(
      http.get(definitionURL, () =>
        HttpResponse.json({ error: { code: 'not_implemented', message: 'no' } }, { status: 501 }),
      ),
    )
    renderSection(vmFor({ ref, descriptors: [] }))
    await waitFor(() => expect(screen.getByText('Could not load definition.')).toBeInTheDocument())
  })
})
