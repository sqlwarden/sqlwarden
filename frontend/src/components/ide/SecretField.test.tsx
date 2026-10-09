import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useReducer } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { reduceSecret, type SecretFieldState } from './secretField'
import { SecretField } from './SecretField'

function Harness({
  initial,
  reveal,
  multiline,
}: {
  initial: SecretFieldState
  reveal?: () => Promise<string>
  multiline?: boolean
}) {
  const [state, dispatch] = useReducer(reduceSecret, initial)
  return (
    <>
      <SecretField
        binding={{ state, dispatch, reveal }}
        noun="password"
        label="Password"
        multiline={multiline}
      />
      <output data-testid="kind">{state.kind}</output>
    </>
  )
}

describe('SecretField', () => {
  it('shows Saved for a stored secret without a show button when it is not revealable', () => {
    render(<Harness initial={{ kind: 'saved', revealable: false }} />)
    expect(screen.getByPlaceholderText('Saved')).toHaveValue('')
    expect(screen.queryByRole('button', { name: /show saved/i })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /remove saved password/i })).toBeInTheDocument()
  })

  it('reveals the saved value on demand and hides it again', async () => {
    const reveal = vi.fn().mockResolvedValue('hunter2')
    render(<Harness initial={{ kind: 'saved', revealable: true }} reveal={reveal} />)

    await userEvent.click(screen.getByRole('button', { name: /show saved password/i }))
    await waitFor(() => expect(screen.getByLabelText('Password')).toHaveValue('hunter2'))
    expect(screen.getByLabelText('Password')).toHaveAttribute('type', 'text')

    await userEvent.click(screen.getByRole('button', { name: /hide saved password/i }))
    expect(screen.getByLabelText('Password')).toHaveValue('')
    expect(screen.getByLabelText('Password')).toHaveAttribute('type', 'password')
    expect(reveal).toHaveBeenCalledTimes(1)
  })

  it('shows managed externally without any show or edit controls', () => {
    render(<Harness initial={{ kind: 'managed', source: 'reference' }} reveal={vi.fn()} />)
    const input = screen.getByLabelText('Password')
    expect(input).toHaveValue('Managed externally')
    expect(input).toBeDisabled()
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })

  it('replaces a saved secret when typed into', async () => {
    render(<Harness initial={{ kind: 'saved', revealable: false }} />)
    await userEvent.type(screen.getByLabelText('Password'), 'abc')
    expect(screen.getByTestId('kind')).toHaveTextContent('replaced')
    expect(screen.getByLabelText('Password')).toHaveValue('abc')
  })

  it('removes a saved secret and lets it be kept again', async () => {
    render(<Harness initial={{ kind: 'saved', revealable: false }} />)
    await userEvent.click(screen.getByRole('button', { name: /remove saved password/i }))
    expect(screen.getByTestId('kind')).toHaveTextContent('cleared')
    expect(screen.getByLabelText('Password')).toBeDisabled()
    expect(screen.getByText(/saved password will be removed on save/i)).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: /keep it/i }))
    expect(screen.getByTestId('kind')).toHaveTextContent('saved')
    expect(screen.getByLabelText('Password')).toBeEnabled()
  })

  it('toggles the visibility of a typed value', async () => {
    render(<Harness initial={{ kind: 'empty' }} />)
    const input = screen.getByLabelText('Password')
    expect(input).toHaveAttribute('type', 'password')
    await userEvent.click(screen.getByRole('button', { name: /show password/i }))
    expect(input).toHaveAttribute('type', 'text')
  })

  it('offers no removal for a secret that was never stored', async () => {
    render(<Harness initial={{ kind: 'empty' }} />)
    expect(screen.queryByRole('button', { name: /remove saved/i })).not.toBeInTheDocument()
    await userEvent.type(screen.getByLabelText('Password'), 'x')
    expect(screen.queryByRole('button', { name: /remove saved/i })).not.toBeInTheDocument()
  })

  it('renders a textarea for multiline secrets', () => {
    render(<Harness initial={{ kind: 'empty' }} multiline />)
    expect(screen.getByLabelText('Password').tagName).toBe('TEXTAREA')
  })
})
