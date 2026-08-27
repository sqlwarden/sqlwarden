import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { ExplainAnalyzeConfirmDialog } from './ExplainAnalyzeConfirmDialog'

describe('ExplainAnalyzeConfirmDialog', () => {
  it('renders the submitted SQL and an execution warning', () => {
    render(
      <ExplainAnalyzeConfirmDialog
        open
        onOpenChange={vi.fn()}
        sql="DELETE FROM widgets"
        onConfirm={vi.fn()}
      />,
    )
    expect(screen.getByRole('alertdialog')).toHaveTextContent('DELETE FROM widgets')
    expect(screen.getByRole('alertdialog')).toHaveTextContent(/executes the statement/i)
  })

  it('calls onOpenChange(false) when Cancel is clicked, without calling onConfirm', async () => {
    const user = userEvent.setup()
    const onOpenChange = vi.fn()
    const onConfirm = vi.fn()
    render(
      <ExplainAnalyzeConfirmDialog
        open
        onOpenChange={onOpenChange}
        sql="DELETE FROM widgets"
        onConfirm={onConfirm}
      />,
    )
    await user.click(screen.getByRole('button', { name: /cancel/i }))
    expect(onOpenChange).toHaveBeenCalledWith(false, expect.anything())
    expect(onConfirm).not.toHaveBeenCalled()
  })

  it('calls onConfirm when Explain Analyze is clicked', async () => {
    const user = userEvent.setup()
    const onConfirm = vi.fn()
    render(
      <ExplainAnalyzeConfirmDialog
        open
        onOpenChange={vi.fn()}
        sql="DELETE FROM widgets"
        onConfirm={onConfirm}
      />,
    )
    await user.click(screen.getByRole('button', { name: /explain analyze/i }))
    expect(onConfirm).toHaveBeenCalled()
  })
})
