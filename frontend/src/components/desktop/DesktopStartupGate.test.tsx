import { screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { renderWithProviders } from '#/test/render'
import { useDesktopRuntime } from '#/lib/desktop/context'
import { DesktopStartupGate } from './DesktopStartupGate'
import { configurePersistentAccessTokens, getAccessToken } from '#/lib/auth/access-token'

afterEach(() => {
  delete window.go
  configurePersistentAccessTokens()
})

function desktopSession() {
  return {
    access_token: 'desktop-token',
    auth_session_id: 'session-1',
    identity: { account_id: 1, org_id: 2, org_slug: 'local', workspace_id: 3 },
  }
}

function RuntimeConsumer() {
  const runtime = useDesktopRuntime()
  return <div>{runtime.session?.identity.org_slug ?? 'server child'}</div>
}

describe('DesktopStartupGate', () => {
  it('passes server children through without invoking a native bridge', () => {
    renderWithProviders(
      <DesktopStartupGate>
        <RuntimeConsumer />
      </DesktopStartupGate>,
    )
    expect(screen.getByText('server child')).toBeInTheDocument()
  })

  it('holds native routes until the local session is ready', async () => {
    let resolveSession!: (value: ReturnType<typeof desktopSession>) => void
    const pending = new Promise<ReturnType<typeof desktopSession>>((resolve) => {
      resolveSession = resolve
    })
    window.go = {
      main: {
        DesktopBridge: {
          StartSession: vi.fn(() => pending),
          GetInfo: vi.fn(async () => ({ version: 'test', paths: emptyPaths() })),
          RevealDataDirectory: vi.fn(async () => undefined),
          RevealLogDirectory: vi.fn(async () => undefined),
        },
      },
    }

    renderWithProviders(
      <DesktopStartupGate>
        <RuntimeConsumer />
      </DesktopStartupGate>,
    )
    expect(screen.getByLabelText('Starting SQLWarden')).toBeInTheDocument()
    expect(screen.queryByText('local')).not.toBeInTheDocument()

    resolveSession(desktopSession())
    expect(await screen.findByText('local')).toBeInTheDocument()
    expect(getAccessToken()).toBe('desktop-token')
    expect(localStorage.getItem('sqlwarden.access_token')).toBeNull()
  })

  it('shows actionable native startup errors', async () => {
    const revealData = vi.fn(async () => undefined)
    const revealLogs = vi.fn(async () => undefined)
    window.go = {
      main: {
        DesktopBridge: {
          StartSession: vi.fn(async () => {
            throw new Error('database is locked')
          }),
          GetInfo: vi.fn(async () => ({
            version: 'test',
            paths: { ...emptyPaths(), data_dir: '/data', logs: '/logs' },
          })),
          RevealDataDirectory: revealData,
          RevealLogDirectory: revealLogs,
        },
      },
    }
    const { user } = renderWithProviders(
      <DesktopStartupGate>
        <RuntimeConsumer />
      </DesktopStartupGate>,
    )

    expect(await screen.findByText('SQLWarden could not start')).toBeInTheDocument()
    expect(screen.getByText('database is locked')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Open data folder' }))
    await user.click(screen.getByRole('button', { name: 'Open logs' }))
    await waitFor(() => {
      expect(revealData).toHaveBeenCalledOnce()
      expect(revealLogs).toHaveBeenCalledOnce()
    })
  })
})

function emptyPaths() {
  return { data_dir: '', database: '', files: '', logs: '', config_file: '' }
}
