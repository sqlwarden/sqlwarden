import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { SessionResponse } from '#/lib/api/types'
import { setAccessToken, getAccessToken } from '#/lib/auth/access-token'
import { SidebarProvider } from '#/components/ui/sidebar'
import { createTestQueryClient } from '#/test/render'
import { server } from '#/test/server'
import { AppShellUserMenu } from './app-shell'

const router = vi.hoisted(() => ({ navigate: vi.fn(() => Promise.resolve()) }))
vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  const React = await import('react')
  return {
    ...actual,
    Link: React.forwardRef<
      HTMLAnchorElement,
      { to: string; children?: React.ReactNode } & React.AnchorHTMLAttributes<HTMLAnchorElement>
    >(({ to, children, ...props }, ref) => (
      <a ref={ref} href={to} {...props}>
        {children}
      </a>
    )),
    useNavigate: () => router.navigate,
  }
})

const session: SessionResponse = {
  account: { id: 1, name: 'Ada Lovelace', email: 'ada@example.com', is_active: true },
  organizations: [{ id: 1, slug: 'acme', name: 'Acme', created_at: '', updated_at: '' }],
  is_instance_admin: false,
  personal_spaces_enabled: false,
}

describe('AppShellUserMenu', () => {
  beforeEach(() => vi.clearAllMocks())

  it('clears authentication state and redirects even when logout fails', async () => {
    server.use(
      http.post('/api/v1/auth/logout', () =>
        HttpResponse.json({ error: { message: 'Unavailable' } }, { status: 503 }),
      ),
    )
    setAccessToken('token')
    const queryClient = createTestQueryClient()
    queryClient.setQueryData(['permissions', 'acme'], ['org:read'])
    const user = userEvent.setup()

    render(
      <QueryClientProvider client={queryClient}>
        <SidebarProvider>
          <AppShellUserMenu session={session} />
        </SidebarProvider>
      </QueryClientProvider>,
    )

    await user.click(screen.getByRole('button', { name: /Ada Lovelace/ }))
    await user.click(await screen.findByRole('menuitem', { name: 'Sign out' }))

    await waitFor(() =>
      expect(router.navigate).toHaveBeenCalledWith({ to: '/login', replace: true }),
    )
    expect(getAccessToken()).toBeNull()
    expect(queryClient.getQueryData(['permissions', 'acme'])).toBeUndefined()
  })
})
