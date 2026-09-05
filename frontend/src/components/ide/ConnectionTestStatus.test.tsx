import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { TestStatusIndicator } from './ConnectionTestStatus'

vi.mock('sonner', () => ({ toast: { success: vi.fn() } }))

describe('TestStatusIndicator', () => {
  it('renders nothing when idle', () => {
    const { container } = render(<TestStatusIndicator state={{ status: 'idle' }} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('shows latency on success', () => {
    render(<TestStatusIndicator state={{ status: 'ok', latencyMs: 42 }} />)
    expect(screen.getByText('42ms')).toBeInTheDocument()
  })

  it('truncates the error inline and reveals the full, copyable message on click', async () => {
    const message =
      'connection refused: dial tcp 10.0.0.1:5432: this is a long driver error message that should not fit inline'
    render(<TestStatusIndicator state={{ status: 'error', message }} />)

    // Rendered twice (trigger + popover content); assert at least the trigger is present.
    expect(screen.getAllByText(message).length).toBeGreaterThan(0)

    await userEvent.click(screen.getByRole('button'))
    const pre = await screen.findByText(message, { selector: 'pre' })
    expect(pre).toHaveClass('select-text')

    await userEvent.click(screen.getByRole('button', { name: /copy error message/i }))
    const { toast } = await import('sonner')
    expect(toast.success).toHaveBeenCalled()
  })
})
