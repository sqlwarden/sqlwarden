import { Suspense, lazy, useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Outlet, createRootRoute, useRouter } from '@tanstack/react-router'
import { Toaster } from 'sonner'
import { GlobalLoadingBar } from '#/components/GlobalLoadingBar'
import { ThemeProvider } from '#/components/theme-provider'
import { TooltipProvider } from '#/components/ui/tooltip'
import { IconPackProvider } from '#/lib/icons'
import { EditorThemeProvider } from '#/lib/editor-themes/context'
import { EditorFontProvider } from '#/lib/editor-font/context'
import { HeadingFontProvider } from '#/lib/heading-font/context'
import { InterfaceFontProvider } from '#/lib/interface-font/context'
import { ThemeLabProvider } from '#/lib/theme-lab/context'
import { ConnectionLayoutProvider } from '#/components/ide/useConnectionLayout'
import { clearAuthScopedQueryCache } from '#/lib/auth/query-cache'
import { AUTH_INVALIDATED_EVENT } from '#/lib/auth/invalidation'
import { loginSearchFor } from '#/lib/auth/login-redirect'
import { isNativeDesktop, startDesktopSession } from '#/lib/desktop/runtime'
import '../styles.css'
import { DesktopNativeEvents } from '#/components/desktop/DesktopNativeEvents'

// Dev-only. The import.meta.env.DEV guard lets Rollup drop the devtools
// (and their dependencies) from production bundles entirely.
const Devtools =
  import.meta.env.DEV && import.meta.env.MODE !== 'test'
    ? lazy(() => import('#/components/devtools'))
    : () => null

export const Route = createRootRoute({
  component: RootComponent,
})

function RootComponent() {
  const router = useRouter()
  const queryClient = useQueryClient()

  useEffect(() => {
    async function handleAuthInvalidated() {
      clearAuthScopedQueryCache(queryClient)
      if (isNativeDesktop()) {
        try {
          await startDesktopSession()
          await queryClient.invalidateQueries()
          await router.invalidate()
          return
        } catch {
          // The next request will retain the startup error rather than exposing login.
        }
      }
      void router.navigate({
        to: '/login',
        search: loginSearchFor(router.state.location.href),
        replace: true,
      })
    }

    window.addEventListener(AUTH_INVALIDATED_EVENT, handleAuthInvalidated)
    return () => window.removeEventListener(AUTH_INVALIDATED_EVENT, handleAuthInvalidated)
  }, [queryClient, router])

  return (
    <ThemeProvider defaultTheme="system" storageKey="theme">
      <ThemeLabProvider>
        <IconPackProvider>
          <EditorThemeProvider>
            <EditorFontProvider>
              <HeadingFontProvider>
                <InterfaceFontProvider>
                  <ConnectionLayoutProvider>
                    <TooltipProvider>
                      <GlobalLoadingBar />
                      <DesktopNativeEvents />
                      <div className="flex min-h-screen flex-col">
                        <div className="flex-1">
                          <Outlet />
                        </div>
                      </div>
                    </TooltipProvider>
                  </ConnectionLayoutProvider>
                </InterfaceFontProvider>
              </HeadingFontProvider>
            </EditorFontProvider>
            <Toaster closeButton position="top-center" theme="system" />
            <Suspense fallback={null}>
              <Devtools />
            </Suspense>
          </EditorThemeProvider>
        </IconPackProvider>
      </ThemeLabProvider>
    </ThemeProvider>
  )
}
