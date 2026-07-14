import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ExportButton } from './ExportButton'

const mocks = vi.hoisted(() => ({
  cancel: vi.fn(),
  download: vi.fn(),
  state: { isDownloading: false, bytesDownloaded: 0 },
}))

vi.mock('./useDownloadNow', () => ({
  useDownloadNow: () => ({ ...mocks.state, download: mocks.download, cancel: mocks.cancel }),
}))
vi.mock('./ExportToFilesDialog', () => ({
  ExportToFilesDialog: () => <div role="dialog">Workspace export</div>,
}))

describe('ExportButton', () => {
  beforeEach(() => {
    mocks.cancel.mockReset()
    mocks.download.mockReset().mockResolvedValue(undefined)
    mocks.state.isDownloading = false
    mocks.state.bytesDownloaded = 0
  })

  it('requires a connection and non-empty SQL before downloading', async () => {
    const user = userEvent.setup()
    const rendered = render(
      <ExportButton
        orgSlug="acme"
        workspaceId={3}
        connectionId={undefined}
        getSql={() => 'select 1'}
      />,
    )
    expect(screen.getByRole('button', { name: 'Export' })).toBeDisabled()

    rendered.rerender(
      <ExportButton orgSlug="acme" workspaceId={3} connectionId={7} getSql={() => ''} />,
    )
    await user.click(screen.getByRole('button', { name: 'Export' }))
    expect(mocks.download).not.toHaveBeenCalled()

    rendered.rerender(
      <ExportButton orgSlug="acme" workspaceId={3} connectionId={7} getSql={() => 'select 1'} />,
    )
    await user.click(screen.getByRole('button', { name: 'Export' }))
    expect(mocks.download).toHaveBeenCalledWith(7, 'select 1')
  })

  it('opens workspace export options and cancels an active download', async () => {
    const user = userEvent.setup()
    const rendered = render(
      <ExportButton orgSlug="acme" workspaceId={3} connectionId={7} getSql={() => 'select 1'} />,
    )
    await user.click(screen.getByRole('button', { name: 'More export options' }))
    await user.click(await screen.findByRole('menuitem', { name: 'Export to workspace' }))
    expect(screen.getByRole('dialog')).toHaveTextContent('Workspace export')

    mocks.state.isDownloading = true
    mocks.state.bytesDownloaded = 2048
    rendered.rerender(
      <ExportButton orgSlug="acme" workspaceId={3} connectionId={7} getSql={() => 'select 1'} />,
    )
    expect(screen.getByRole('button', { name: /Exporting/ })).toHaveTextContent('2 KB')
    await user.click(screen.getByRole('button', { name: 'Cancel export' }))
    expect(mocks.cancel).toHaveBeenCalled()
  })
})
