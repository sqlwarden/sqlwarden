import { describe, expect, it, vi } from 'vitest'
import {
  availableCommands,
  commandForKeyboardEvent,
  executeCommand,
  registerCommand,
} from './registry'

describe('command registry', () => {
  it('publishes and executes only registered commands', async () => {
    const handler = vi.fn()
    const unregister = registerCommand('app.settings', handler)
    expect(availableCommands().map((command) => command.id)).toContain('app.settings')
    await executeCommand('app.settings')
    expect(handler).toHaveBeenCalledOnce()
    unregister()
    expect(availableCommands().map((command) => command.id)).not.toContain('app.settings')
  })

  it('maps shared desktop and browser shortcuts', () => {
    const event = new KeyboardEvent('keydown', { key: 'P', ctrlKey: true, shiftKey: true })
    expect(commandForKeyboardEvent(event)).toBe('view.command-palette')
    expect(commandForKeyboardEvent(new KeyboardEvent('keydown', { key: 'o', metaKey: true }))).toBe(
      'file.open-native',
    )
  })
})
