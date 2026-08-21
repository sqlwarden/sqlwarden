import { screen } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import { setAccessToken } from '#/lib/auth/access-token'
import { renderRoute } from '#/test/render'
import { server } from '#/test/server'
import { sessionHandler, setupStatusHandler } from '#/test/handlers'
import { sessionFixture } from '#/test/fixtures'

describe('SettingsApiTokensPage', () => {
  beforeEach(() => {
    setAccessToken('test-token')
    server.use(setupStatusHandler(), sessionHandler(sessionFixture()))
  })

  it('shows a Coming soon empty state instead of the scaffold placeholder text', async () => {
    renderRoute('/settings/api-tokens')

    expect(await screen.findByText(/Coming soon/)).toBeInTheDocument()
    expect(screen.queryByText('API Tokens works!')).not.toBeInTheDocument()
    expect(screen.getByText('API Tokens', { selector: 'p' })).toBeInTheDocument()
  })
})
