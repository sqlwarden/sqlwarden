import { useEffect, useState, useSyncExternalStore } from 'react'
import { useRouter } from '@tanstack/react-router'
import type { NativeTextFile } from '#/lib/desktop/runtime'
import { drainDesktopOpenRequests, isNativeDesktop } from '#/lib/desktop/runtime'
import { platformService } from '#/lib/platform/service'
import {
  availableCommands,
  commandForKeyboardEvent,
  executeCommand,
  registerCommand,
  subscribeCommands,
  type CommandId,
} from '#/lib/commands/registry'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '#/components/ui/dialog'
import { Input } from '#/components/ui/input'

declare global {
  interface Window {
    runtime?: {
      EventsOn(event: string, callback: (...data: unknown[]) => void): () => void
    }
  }
}

export const NATIVE_FILE_OPENED_EVENT = 'sqlwarden:native-file-opened'
export const NATIVE_SQLITE_SELECTED_EVENT = 'sqlwarden:native-sqlite-selected'
const pendingFiles: NativeTextFile[] = []
const pendingSQLiteFiles: string[] = []

export function takePendingNativeFiles() {
  return pendingFiles.splice(0)
}

export function claimPendingNativeFile(file: NativeTextFile) {
  const index = pendingFiles.indexOf(file)
  if (index >= 0) pendingFiles.splice(index, 1)
}

export function takePendingSQLiteFiles() {
  return pendingSQLiteFiles.splice(0)
}

export function claimPendingSQLiteFile(path: string) {
  const index = pendingSQLiteFiles.indexOf(path)
  if (index >= 0) pendingSQLiteFiles.splice(index, 1)
}

export function DesktopNativeEvents() {
  const router = useRouter()
  const [paletteOpen, setPaletteOpen] = useState(false)
  const [query, setQuery] = useState('')
  const commands = useSyncExternalStore(subscribeCommands, availableCommands, availableCommands)

  useEffect(() => {
    const cleanups: Array<() => void> = []
    cleanups.push(
      registerCommand('app.settings', () => router.navigate({ to: '/desktop/settings' })),
      registerCommand('view.command-palette', () => setPaletteOpen(true)),
      registerCommand('file.open-native', async () => {
        const file = await platformService().openSQLFile()
        if (!file) return
        pendingFiles.push(file)
        window.dispatchEvent(new CustomEvent(NATIVE_FILE_OPENED_EVENT, { detail: file }))
      }),
      registerCommand('app.check-updates', () =>
        platformService().openExternalURL('https://github.com/sqlwarden/sqlwarden/releases/latest'),
      ),
    )
    if (!isNativeDesktop()) {
      const handleShortcut = (event: KeyboardEvent) => {
        const command = commandForKeyboardEvent(event)
        if (!command || command === 'file.save' || command === 'file.save-as') return
        event.preventDefault()
        void executeCommand(command)
      }
      window.addEventListener('keydown', handleShortcut)
      return () => {
        cleanups.forEach((cleanup) => cleanup())
        window.removeEventListener('keydown', handleShortcut)
      }
    }
    const on = window.runtime?.EventsOn
    if (on) {
      cleanups.push(
        on('desktop:command', (command) => {
          if (typeof command === 'string') void executeCommand(command as CommandId)
        }),
        on('desktop:file-opened', (file) => {
          pendingFiles.push(file as NativeTextFile)
          window.dispatchEvent(
            new CustomEvent<NativeTextFile>(NATIVE_FILE_OPENED_EVENT, {
              detail: file as NativeTextFile,
            }),
          )
        }),
        on('desktop:sqlite-selected', (path) => {
          pendingSQLiteFiles.push(String(path))
          window.dispatchEvent(
            new CustomEvent(NATIVE_SQLITE_SELECTED_EVENT, { detail: String(path) }),
          )
        }),
      )
    }
    void drainDesktopOpenRequests().then((requests) => {
      requests?.files.forEach((file) => {
        pendingFiles.push(file)
        window.dispatchEvent(new CustomEvent(NATIVE_FILE_OPENED_EVENT, { detail: file }))
      })
      requests?.sqlite_files.forEach((path) => {
        pendingSQLiteFiles.push(path)
        window.dispatchEvent(new CustomEvent(NATIVE_SQLITE_SELECTED_EVENT, { detail: path }))
      })
    })

    function handleExternalLink(event: MouseEvent) {
      const anchor = (event.target as Element | null)?.closest(
        'a[href]',
      ) as HTMLAnchorElement | null
      if (!anchor || !/^https?:/.test(anchor.href)) return
      event.preventDefault()
      void platformService().openExternalURL(anchor.href)
    }
    document.addEventListener('click', handleExternalLink)
    return () => {
      cleanups.forEach((cleanup) => cleanup())
      document.removeEventListener('click', handleExternalLink)
    }
  }, [router])

  const filtered = commands.filter((command) =>
    command.label.toLowerCase().includes(query.trim().toLowerCase()),
  )
  return (
    <Dialog
      open={paletteOpen}
      onOpenChange={(open) => {
        setPaletteOpen(open)
        if (!open) setQuery('')
      }}
    >
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Command palette</DialogTitle>
        </DialogHeader>
        <Input
          autoFocus
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          placeholder="Search commands…"
          aria-label="Search commands"
        />
        <div className="max-h-80 overflow-y-auto" role="listbox" aria-label="Commands">
          {filtered.map((command) => (
            <button
              key={command.id}
              type="button"
              role="option"
              aria-selected="false"
              className="flex h-9 w-full items-center justify-between rounded-md px-2 text-left text-sm hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              onClick={() => {
                setPaletteOpen(false)
                void executeCommand(command.id)
              }}
            >
              <span>{command.label}</span>
              {command.shortcut ? (
                <kbd className="text-xs text-muted-foreground">{command.shortcut}</kbd>
              ) : null}
            </button>
          ))}
          {filtered.length === 0 ? (
            <p className="py-6 text-center text-sm text-muted-foreground">No commands found.</p>
          ) : null}
        </div>
      </DialogContent>
    </Dialog>
  )
}
