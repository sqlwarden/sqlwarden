import { screen } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it, vi } from 'vitest'
import { renderWithProviders } from '#/test/render'
import { server } from '#/test/server'
import { GenerateStatementDialog } from './GenerateStatementDialog'

const { copyWithToastMock } = vi.hoisted(() => ({ copyWithToastMock: vi.fn() }))

vi.mock('./contextMenus/clipboard', () => ({ copyWithToast: copyWithToastMock }))

vi.mock('./object-detail/ReadOnlySqlView', () => ({
  ReadOnlySqlView: ({ value }: { value: string }) => (
    <pre data-testid="generated-sql-preview">{value}</pre>
  ),
}))

const endpoint = '/api/v1/orgs/acme/workspaces/3/connections/7/schema/statements'
const target = {
  operation: 'select' as const,
  ref: { scope: [{ kind: 'schema', name: 'public' }], kind: 'table', name: 'orders' },
}

function renderDialog(sessionId?: string) {
  return renderWithProviders(
    <GenerateStatementDialog
      open
      onOpenChange={vi.fn()}
      orgSlug="acme"
      workspaceId={3}
      connectionId={7}
      sessionId={sessionId}
      target={target}
    />,
  )
}

describe('GenerateStatementDialog', () => {
  it('shows loading, previews the generated SQL, and copies it', async () => {
    let resolveResponse: ((response: Response) => void) | undefined
    const pending = new Promise<Response>((resolve) => {
      resolveResponse = resolve
    })
    let sessionHeader: string | null = null
    server.use(
      http.post(endpoint, ({ request }) => {
        sessionHeader = request.headers.get('X-Warden-Session')
        return pending
      }),
    )
    const { user } = renderDialog('session-7')

    expect(await screen.findByText('Generating SQL…')).toBeInTheDocument()
    resolveResponse?.(HttpResponse.json({ sql: 'SELECT "id" FROM "public"."orders";' }))

    expect(await screen.findByTestId('generated-sql-preview')).toHaveTextContent(
      'SELECT "id" FROM "public"."orders";',
    )
    expect(sessionHeader).toBe('session-7')
    await user.click(screen.getByRole('button', { name: 'Copy' }))
    expect(copyWithToastMock).toHaveBeenCalledWith(
      'SELECT "id" FROM "public"."orders";',
      'SQL copied',
    )
  })

  it('can generate without a session and retries a failed request', async () => {
    let requests = 0
    let sessionHeader: string | null = 'not-called'
    server.use(
      http.post(endpoint, ({ request }) => {
        requests++
        sessionHeader = request.headers.get('X-Warden-Session')
        if (requests === 1) {
          return HttpResponse.json({ message: 'Generation failed.' }, { status: 500 })
        }
        return HttpResponse.json({ sql: 'SELECT "id" FROM "public"."orders";' })
      }),
    )
    const { user } = renderDialog()

    expect(await screen.findByRole('button', { name: 'Retry' })).toBeInTheDocument()
    expect(sessionHeader).toBeNull()
    await user.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByTestId('generated-sql-preview')).toHaveTextContent('SELECT "id"')
    expect(requests).toBe(2)
  })
})
