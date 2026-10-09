import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useEffect, useState } from 'react'
import { describe, expect, it, vi } from 'vitest'
import type { ConnectionSecretViews } from '#/lib/api/types'
import { ConnectionTlsFields, emptyTlsState, type TlsFormState } from './ConnectionTlsFields'
import { standardTlsSpec } from './engines/tls'
import { useSecretFields } from './useSecretFields'

const storedKey: ConnectionSecretViews = {
  tls_client_key: { set: true, source: 'stored', revealable: false },
}

function Harness({
  initial,
  secrets,
  spec = standardTlsSpec,
}: {
  initial?: Partial<TlsFormState>
  secrets?: ConnectionSecretViews
  spec?: typeof standardTlsSpec
}) {
  const [value, setValue] = useState<TlsFormState>({ ...emptyTlsState, ...initial })
  const { bind, load } = useSecretFields()
  useEffect(() => load(secrets), [load, secrets])
  return (
    <ConnectionTlsFields
      spec={spec}
      value={value}
      disabled={false}
      bindSecret={bind}
      onChange={setValue}
    />
  )
}

describe('ConnectionTlsFields', () => {
  it('renders nothing without a spec', () => {
    const { container } = render(
      <ConnectionTlsFields
        spec={undefined}
        value={emptyTlsState}
        disabled={false}
        bindSecret={vi.fn()}
        onChange={vi.fn()}
      />,
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('enables PEM textareas when TLS is on', () => {
    render(<Harness initial={{ mode: 'verify-full' }} />)
    expect(screen.getByLabelText(/tls mode/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/ca bundle/i)).toBeEnabled()
    expect(screen.getByLabelText(/client certificate/i)).toBeEnabled()
    expect(screen.getByLabelText(/client key/i)).toBeEnabled()
    expect(screen.getByLabelText(/server name/i)).toBeEnabled()
  })

  it('keeps PEM inputs mounted but disabled when mode is disable', () => {
    render(<Harness />)
    expect(screen.getByLabelText(/tls mode/i)).toBeEnabled()
    expect(screen.getByLabelText(/ca bundle/i)).toBeDisabled()
    expect(screen.getByLabelText(/client key/i)).toBeDisabled()
    expect(screen.getByText('CA bundle (PEM)').parentElement).toHaveAttribute('data-disabled')
  })

  it('shows a saved client key without its value', () => {
    render(<Harness initial={{ mode: 'verify-full' }} secrets={storedKey} />)
    expect(screen.getByPlaceholderText('Saved')).toHaveValue('')
  })

  it('removes a saved client key on request and lets it be restored', async () => {
    render(<Harness initial={{ mode: 'verify-full' }} secrets={storedKey} />)
    await userEvent.click(screen.getByRole('button', { name: /remove saved client key/i }))
    expect(screen.getByLabelText(/client key/i)).toBeDisabled()
    expect(screen.getByText(/saved client key will be removed on save/i)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: /keep it/i }))
    expect(screen.getByLabelText(/client key/i)).toBeEnabled()
  })
})
