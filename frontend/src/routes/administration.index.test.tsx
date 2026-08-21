import { screen } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import { setAccessToken } from '#/lib/auth/access-token'
import { renderRoute } from '#/test/render'
import { server } from '#/test/server'
import { sessionHandler, setupStatusHandler } from '#/test/handlers'
import { sessionFixture } from '#/test/fixtures'

describe('AdministrationIndexPage', () => {
  beforeEach(() => {
    setAccessToken('test-token')
    server.use(setupStatusHandler(), sessionHandler(sessionFixture({ is_instance_admin: true })))
  })

  it('renders overview cards instead of redirecting to Users', async () => {
    renderRoute('/administration')

    expect(await screen.findByText('Every account on this instance.')).toBeInTheDocument()
    expect(screen.getByText('All organizations hosted on this instance.')).toBeInTheDocument()
    expect(screen.getByText('Instance-wide users, orgs, and settings.')).toBeInTheDocument()
  })
})
