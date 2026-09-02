import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ConnectionTlsFields, emptyTlsState } from './ConnectionTlsFields'
import { standardTlsSpec } from './engines/tls'

describe('ConnectionTlsFields', () => {
  it('renders nothing without a spec', () => {
    const { container } = render(
      <ConnectionTlsFields
        spec={undefined}
        value={emptyTlsState}
        disabled={false}
        onChange={vi.fn()}
      />,
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('enables PEM textareas when TLS is on', () => {
    render(
      <ConnectionTlsFields
        spec={standardTlsSpec}
        value={{ ...emptyTlsState, mode: 'verify-full' }}
        disabled={false}
        onChange={vi.fn()}
      />,
    )
    expect(screen.getByLabelText(/tls mode/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/ca bundle/i)).toBeEnabled()
    expect(screen.getByLabelText(/client certificate/i)).toBeEnabled()
    expect(screen.getByLabelText(/client key/i)).toBeEnabled()
    expect(screen.getByLabelText(/server name/i)).toBeEnabled()
  })

  it('keeps PEM inputs mounted but disabled when mode is disable', () => {
    render(
      <ConnectionTlsFields
        spec={standardTlsSpec}
        value={emptyTlsState}
        disabled={false}
        onChange={vi.fn()}
      />,
    )
    expect(screen.getByLabelText(/tls mode/i)).toBeEnabled()
    expect(screen.getByLabelText(/ca bundle/i)).toBeDisabled()
    expect(screen.getByLabelText(/client key/i)).toBeDisabled()
    // The field wrapper carries data-disabled so the label mutes with the input.
    expect(screen.getByText('CA bundle (PEM)').parentElement).toHaveAttribute('data-disabled')
  })

  it('shows stored-key placeholder in edit mode', () => {
    render(
      <ConnectionTlsFields
        spec={standardTlsSpec}
        value={{ ...emptyTlsState, mode: 'verify-full', clientKeySet: true }}
        disabled={false}
        onChange={vi.fn()}
      />,
    )
    expect(screen.getByPlaceholderText(/leave blank to keep/i)).toBeInTheDocument()
  })
})
