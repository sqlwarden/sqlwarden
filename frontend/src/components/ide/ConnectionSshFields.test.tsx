import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useEffect, useState } from 'react'
import { describe, expect, it } from 'vitest'

import type { ConnectionSecretViews } from '#/lib/api/types'
import { ConnectionSshFields, emptySshState, type SshFormState } from './ConnectionSshFields'
import { useSecretFields } from './useSecretFields'

const storedPassword: ConnectionSecretViews = {
  ssh_password: { set: true, source: 'stored', revealable: false },
}
const storedKey: ConnectionSecretViews = {
  ssh_private_key: { set: true, source: 'stored', revealable: false },
}

function Harness({
  initial,
  secrets,
}: {
  initial?: Partial<SshFormState>
  secrets?: ConnectionSecretViews
} = {}) {
  const [value, setValue] = useState<SshFormState>({ ...emptySshState, ...initial })
  const { bind, load } = useSecretFields()
  useEffect(() => load(secrets), [load, secrets])
  return <ConnectionSshFields value={value} bindSecret={bind} onChange={setValue} />
}

describe('ConnectionSshFields', () => {
  it('keeps tunnel fields mounted but disabled until enabled', async () => {
    render(<Harness />)
    expect(screen.getByLabelText(/SSH host/i)).toBeDisabled()
    // The field wrapper carries data-disabled so the label mutes with the input.
    expect(screen.getByText('SSH host').parentElement).toHaveAttribute('data-disabled')
    await userEvent.click(screen.getByRole('checkbox', { name: /use ssh tunnel/i }))
    expect(screen.getByLabelText(/SSH host/i)).toBeEnabled()
    expect(screen.getByText('SSH host').parentElement).not.toHaveAttribute('data-disabled')
  })

  it('enables host/user/auth fields when the tunnel is turned on', async () => {
    render(<Harness />)
    await userEvent.click(screen.getByRole('checkbox', { name: /use ssh tunnel/i }))
    expect(screen.getByLabelText(/SSH host/i)).toBeEnabled()
    expect(screen.getByLabelText(/SSH user/i)).toBeEnabled()
  })

  it('swaps password and private key inputs with the auth method', async () => {
    render(<Harness />)
    await userEvent.click(screen.getByRole('checkbox', { name: /use ssh tunnel/i }))
    expect(screen.getByLabelText(/SSH password/i)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('combobox', { name: /authentication/i }))
    await userEvent.click(screen.getByRole('option', { name: /private key/i }))
    expect(screen.getByLabelText(/private key/i)).toBeInTheDocument()
    expect(screen.queryByLabelText(/SSH password/i)).not.toBeInTheDocument()
  })

  it('toggles password visibility from the reveal control', async () => {
    render(<Harness />)
    await userEvent.click(screen.getByRole('checkbox', { name: /use ssh tunnel/i }))
    const password = screen.getByLabelText(/SSH password/i)
    expect(password).toHaveAttribute('type', 'password')
    await userEvent.click(screen.getByRole('button', { name: /show password/i }))
    expect(password).toHaveAttribute('type', 'text')
  })

  it('defaults to skipping host-key verification and reveals the note when opted back in', async () => {
    render(<Harness />)
    await userEvent.click(screen.getByRole('checkbox', { name: /use ssh tunnel/i }))
    expect(screen.queryByLabelText(/known_hosts entry/i)).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('checkbox', { name: /do not verify host key/i }))
    expect(screen.getByLabelText(/known_hosts entry/i)).toBeInTheDocument()
  })

  it('removes a saved password on request and lets it be restored', async () => {
    render(<Harness initial={{ enabled: true, authMethod: 'password' }} secrets={storedPassword} />)
    await userEvent.click(screen.getByRole('button', { name: /remove saved password/i }))
    expect(screen.getByLabelText(/SSH password/i)).toBeDisabled()
    expect(screen.getByText(/saved password will be removed on save/i)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: /keep it/i }))
    expect(screen.getByLabelText(/SSH password/i)).toBeEnabled()
  })

  it('offers removal of a saved private key', async () => {
    render(<Harness initial={{ enabled: true, authMethod: 'private_key' }} secrets={storedKey} />)
    await userEvent.click(screen.getByRole('button', { name: /remove saved key/i }))
    expect(screen.getByLabelText(/private key/i)).toBeDisabled()
  })
})
