import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { Dialog, DialogContent } from './dialog'

describe('DialogContent Enter-to-submit', () => {
  it('submits the enclosing form when Enter is pressed in a text input', async () => {
    const handleSubmit = vi.fn((event: React.FormEvent) => event.preventDefault())
    const user = userEvent.setup()
    render(
      <Dialog open>
        <DialogContent>
          <form onSubmit={handleSubmit}>
            <input aria-label="Name" />
            <button type="submit">Save</button>
          </form>
        </DialogContent>
      </Dialog>,
    )

    await user.type(screen.getByLabelText('Name'), 'Acme{Enter}')

    expect(handleSubmit).toHaveBeenCalledOnce()
  })

  it('does not submit on Enter inside a textarea', async () => {
    const handleSubmit = vi.fn((event: React.FormEvent) => event.preventDefault())
    const user = userEvent.setup()
    render(
      <Dialog open>
        <DialogContent>
          <form onSubmit={handleSubmit}>
            <textarea aria-label="Notes" />
          </form>
        </DialogContent>
      </Dialog>,
    )

    await user.type(screen.getByLabelText('Notes'), 'line one{Enter}line two')

    expect(handleSubmit).not.toHaveBeenCalled()
  })
})
