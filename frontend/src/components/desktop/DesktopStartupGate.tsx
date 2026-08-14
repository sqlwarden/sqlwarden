import { useEffect, useState, type PropsWithChildren } from 'react'
import { Button } from '#/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '#/components/ui/card'
import { Skeleton } from '#/components/ui/skeleton'
import { useBrand } from '#/lib/brand/brand'
import { DesktopRuntimeContext } from '#/lib/desktop/context'
import {
  getDesktopInfo,
  isNativeDesktop,
  revealDesktopDataDirectory,
  revealDesktopLogDirectory,
  startDesktopSession,
  type DesktopInfo,
  type DesktopSession,
} from '#/lib/desktop/runtime'
import { Icon } from '#/lib/icons'

type StartupState =
  | { status: 'server' }
  | { status: 'loading' }
  | { status: 'ready'; session: DesktopSession }
  | { status: 'error'; message: string; info?: DesktopInfo }

export function DesktopStartupGate({ children }: PropsWithChildren) {
  const [state, setState] = useState<StartupState>(() =>
    isNativeDesktop() ? { status: 'loading' } : { status: 'server' },
  )

  useEffect(() => {
    if (state.status !== 'loading') return
    let active = true

    void Promise.allSettled([startDesktopSession(), getDesktopInfo()]).then(
      ([sessionResult, infoResult]) => {
        if (!active) return
        const info = infoResult.status === 'fulfilled' ? infoResult.value : undefined
        if (sessionResult.status === 'fulfilled' && sessionResult.value) {
          setState({ status: 'ready', session: sessionResult.value })
          return
        }
        const reason =
          sessionResult.status === 'rejected'
            ? sessionResult.reason
            : new Error(info?.startup_error || 'The desktop service did not start.')
        setState({
          status: 'error',
          message: reason instanceof Error ? reason.message : String(reason),
          info,
        })
      },
    )
    return () => {
      active = false
    }
  }, [state.status])

  if (state.status === 'server') return children
  if (state.status === 'loading') return <DesktopStarting />
  if (state.status === 'error') return <DesktopStartupFailure state={state} />

  return (
    <DesktopRuntimeContext.Provider value={{ native: true, session: state.session }}>
      {children}
    </DesktopRuntimeContext.Provider>
  )
}

function DesktopStarting() {
  const brand = useBrand()
  return (
    <main className="flex min-h-svh items-center justify-center bg-background px-6">
      <div className="w-full max-w-sm space-y-6" aria-label="Starting SQLWarden">
        <div className="flex justify-center">
          <brand.LogoLockup size={32} />
        </div>
        <Card size="sm">
          <CardHeader>
            <Skeleton className="h-4 w-36" />
            <Skeleton className="h-3 w-56 max-w-full" />
          </CardHeader>
          <CardContent className="space-y-3">
            <Skeleton className="h-8 w-full" />
            <Skeleton className="h-8 w-4/5" />
          </CardContent>
        </Card>
      </div>
    </main>
  )
}

function DesktopStartupFailure({ state }: { state: Extract<StartupState, { status: 'error' }> }) {
  const brand = useBrand()
  return (
    <main
      className="flex min-h-svh items-center justify-center bg-background px-6 py-10"
      aria-live="assertive"
    >
      <div className="w-full max-w-lg space-y-6">
        <brand.LogoLockup size={32} />
        <Card>
          <CardHeader className="border-b border-border">
            <div className="mb-2 flex size-9 items-center justify-center rounded-md bg-destructive/10 text-destructive">
              <Icon name="information-circle" size={20} />
            </div>
            <CardTitle>SQLWarden could not start</CardTitle>
            <CardDescription>
              Your local data was left unchanged. Review the details or open the application folders
              for troubleshooting.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <pre className="max-h-40 overflow-auto rounded-md bg-muted p-3 text-xs whitespace-pre-wrap text-muted-foreground">
              {state.message}
            </pre>
            <div className="flex flex-wrap gap-2">
              {state.info?.paths.data_dir ? (
                <Button variant="outline" onClick={() => void revealDesktopDataDirectory()}>
                  <Icon name="folder-open" size={20} />
                  Open data folder
                </Button>
              ) : null}
              {state.info?.paths.logs ? (
                <Button variant="outline" onClick={() => void revealDesktopLogDirectory()}>
                  <Icon name="folder-open" size={20} />
                  Open logs
                </Button>
              ) : null}
            </div>
          </CardContent>
        </Card>
      </div>
    </main>
  )
}
