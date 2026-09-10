import { describe, expect, it, vi } from 'vitest'
import { executeNativeEditAction, isExternalHttpURL } from './DesktopNativeEvents'

describe('desktop native event helpers', () => {
  it('keeps same-origin Wails routes inside the WebView', () => {
    const current = 'http://wails.localhost/orgs/local/workspaces/3/ide'
    expect(isExternalHttpURL('/desktop/settings', current)).toBe(false)
    expect(isExternalHttpURL('http://wails.localhost/ide/local', current)).toBe(false)
    expect(isExternalHttpURL('http://wails.localhost/ide/local', 'wails://wails.localhost/')).toBe(
      false,
    )
    expect(isExternalHttpURL('https://sqlwarden.com/docs', current)).toBe(true)
    expect(isExternalHttpURL('mailto:support@sqlwarden.com', current)).toBe(false)
  })

  it('dispatches supported native edit actions to the focused WebView document', async () => {
    const original = document.execCommand
    const execCommand = vi.fn(() => true)
    document.execCommand = execCommand

    try {
      await executeNativeEditAction('copy')
      await executeNativeEditAction('unsupported')

      expect(execCommand).toHaveBeenCalledOnce()
      expect(execCommand).toHaveBeenCalledWith('copy')
    } finally {
      document.execCommand = original
    }
  })
})
