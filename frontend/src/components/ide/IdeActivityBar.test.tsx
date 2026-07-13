import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { IdeActivityBar } from './IdeActivityBar'
import { createIdeStore, IdeStoreContext } from './useIdeStore'

describe('IdeActivityBar', () => {
  it('toggles the active sidebar and expands it when switching activities', async () => {
    const store = createIdeStore('acme', 1, 'ephemeral')
    const user = userEvent.setup()
    render(
      <IdeStoreContext.Provider value={store}>
        <IdeActivityBar />
      </IdeStoreContext.Provider>,
    )

    const explorer = screen.getByRole('button', { name: 'Explorer' })
    expect(explorer).toHaveAttribute('aria-pressed', 'true')
    await user.click(explorer)
    expect(store.getState().sidebarCollapsed).toBe(true)

    await user.click(screen.getByRole('button', { name: 'Files' }))
    expect(store.getState().activeActivityId).toBe('files')
    expect(store.getState().sidebarCollapsed).toBe(false)
  })
})
