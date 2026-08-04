import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { AuthProviderDefinition } from '#/lib/auth-providers/types'

const registry = vi.hoisted(() => ({ authProviders: [] as AuthProviderDefinition[] }))
vi.mock('#/lib/auth-providers/registry', () => ({
  get authProviders() {
    return registry.authProviders
  },
}))

import { AuthProviderList } from './AuthProviderList'

function FakeIcon({ className }: { className?: string }) {
  return <svg className={className} />
}

describe('AuthProviderList', () => {
  it('renders nothing when no providers are registered', () => {
    registry.authProviders = []
    const { container } = render(<AuthProviderList />)
    expect(container).toBeEmptyDOMElement()
  })

  it('renders a divider and a button per provider, calling startSignIn with the redirect', () => {
    const startSignIn = vi.fn()
    registry.authProviders = [
      { id: 'google', label: 'Continue with Google', Icon: FakeIcon, startSignIn },
    ]

    render(<AuthProviderList redirect="/workspaces" />)

    expect(screen.getByText('or continue with')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Continue with Google' }))
    expect(startSignIn).toHaveBeenCalledWith('/workspaces')
  })
})
