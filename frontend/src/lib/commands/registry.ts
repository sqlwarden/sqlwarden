export type CommandId =
  | 'app.settings'
  | 'app.check-updates'
  | 'file.open-native'
  | 'file.save'
  | 'file.save-as'
  | 'view.command-palette'
  | 'view.toggle-sidebar'

export interface CommandDefinition {
  id: CommandId
  label: string
  shortcut?: string
}

export const commandDefinitions: readonly CommandDefinition[] = [
  { id: 'file.open-native', label: 'Open SQL file…', shortcut: 'Mod+O' },
  { id: 'file.save', label: 'Save', shortcut: 'Mod+S' },
  { id: 'file.save-as', label: 'Save as…', shortcut: 'Mod+Shift+S' },
  { id: 'view.toggle-sidebar', label: 'Toggle sidebar', shortcut: 'Mod+B' },
  { id: 'app.settings', label: 'Open settings', shortcut: 'Mod+,' },
  { id: 'app.check-updates', label: 'Check for updates…' },
  { id: 'view.command-palette', label: 'Show command palette', shortcut: 'Mod+Shift+P' },
]

type CommandHandler = () => void | Promise<void>
const handlers = new Map<CommandId, CommandHandler>()
const listeners = new Set<() => void>()
let snapshot: readonly CommandDefinition[] = []

function updateSnapshot() {
  snapshot = commandDefinitions.filter((command) => handlers.has(command.id))
  listeners.forEach((listener) => listener())
}

export function registerCommand(id: CommandId, handler: CommandHandler) {
  handlers.set(id, handler)
  updateSnapshot()
  return () => {
    if (handlers.get(id) === handler) handlers.delete(id)
    updateSnapshot()
  }
}

export function executeCommand(id: CommandId) {
  return handlers.get(id)?.()
}

export function availableCommands() {
  return snapshot
}

export function subscribeCommands(listener: () => void) {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

export function commandForKeyboardEvent(event: KeyboardEvent): CommandId | undefined {
  if (!(event.metaKey || event.ctrlKey) || event.altKey) return undefined
  const key = event.key.toLowerCase()
  if (key === 'o' && !event.shiftKey) return 'file.open-native'
  if (key === 's' && event.shiftKey) return 'file.save-as'
  if (key === 's' && !event.shiftKey) return 'file.save'
  if (key === 'b' && !event.shiftKey) return 'view.toggle-sidebar'
  if (key === ',' && !event.shiftKey) return 'app.settings'
  if (key === 'p' && event.shiftKey) return 'view.command-palette'
  return undefined
}
