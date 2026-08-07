import type { PropsWithChildren } from 'react'
import { act, renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import {
  formatPageTitle,
  PageTitleScopeProvider,
  PRODUCT_NAME,
  usePageTitle,
  usePageTitleScope,
} from './page-title'

afterEach(() => {
  document.title = PRODUCT_NAME
})

describe('page titles', () => {
  it('formats normalized context segments and appends the product name once', () => {
    expect(formatPageTitle(' Connections ', '', null, 'Analytics')).toBe(
      'Connections | Analytics | SQLWarden',
    )
    expect(formatPageTitle('Login', 'SQLWarden')).toBe('Login | SQLWarden')
    expect(formatPageTitle()).toBe('SQLWarden')
  })

  it('updates asynchronously and restores the prior title when still owned', () => {
    document.title = PRODUCT_NAME
    const { rerender, unmount } = renderHook(({ name }) => usePageTitle(name, 'Acme'), {
      initialProps: { name: 'User' },
    })
    expect(document.title).toBe('User | Acme | SQLWarden')

    rerender({ name: 'Ada Lovelace' })
    expect(document.title).toBe('Ada Lovelace | Acme | SQLWarden')

    unmount()
    expect(document.title).toBe(PRODUCT_NAME)
  })

  it('does not overwrite a newer title during cleanup', () => {
    const { unmount } = renderHook(() => usePageTitle('Login'))
    act(() => {
      document.title = 'Setup | SQLWarden'
    })
    unmount()
    expect(document.title).toBe('Setup | SQLWarden')
  })

  it('shares loaded organization and workspace names with nested routes', () => {
    function wrapper({ children }: PropsWithChildren) {
      return (
        <PageTitleScopeProvider organizationName="Acme" workspaceName="Analytics">
          {children}
        </PageTitleScopeProvider>
      )
    }
    const { result } = renderHook(() => usePageTitleScope(), { wrapper })
    expect(result.current).toEqual({ organizationName: 'Acme', workspaceName: 'Analytics' })
  })
})
