import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { BindSecret } from './useSecretFields'
import type { ResolvedField } from './connection-drivers/resolveFields'
import { DriverFields } from './ConnectionFormFields'

const bindSecret: BindSecret = () => ({ state: { kind: 'empty' }, dispatch: () => {} })

function renderFields(fields: ResolvedField[]) {
  return render(
    <DriverFields
      fields={fields}
      bindSecret={bindSecret}
      values={{}}
      errors={{}}
      disabled={false}
      onChange={() => {}}
    />,
  )
}

describe('DriverFields', () => {
  afterEach(() => vi.restoreAllMocks())

  it('renders a known secret field as a secret control', () => {
    renderFields([
      { key: 'password', label: 'Password', type: 'string', required: false, secret: true },
    ])

    expect(screen.getByRole('button', { name: 'Show password' })).toBeInTheDocument()
  })

  it('fails loudly for a secret field that has no secret slot', () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})

    expect(() =>
      renderFields([
        { key: 'api_token', label: 'API token', type: 'string', required: false, secret: true },
      ]),
    ).toThrow(/api_token/)
  })
})
