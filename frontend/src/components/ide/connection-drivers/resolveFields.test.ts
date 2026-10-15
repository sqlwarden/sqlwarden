import { describe, expect, it } from 'vitest'
import type { ConnectionFieldSpec } from '#/lib/api/types'
import {
  fieldDefaults,
  paramsFromValues,
  requiredFieldErrors,
  resolveFields,
  valuesFromParams,
} from './resolveFields'

const spec: ConnectionFieldSpec[] = [
  { key: 'host', label: 'Host name', type: 'string', required: true, secret: false },
  { key: 'port', label: 'Port', type: 'int', required: true, default: '5432', secret: false },
  { key: 'password', label: 'Password', type: 'string', required: false, secret: true },
  { key: 'extra', label: 'Extra', type: 'string', required: false, secret: false },
]

describe('resolveFields', () => {
  it('orders by layout, overlays presentation, drops unknown layout keys, and appends unlaid spec fields', () => {
    const fields = resolveFields(spec, [
      { key: 'port', label: 'TCP port', span: 'compact', section: 'Server' },
      { key: 'missing', label: 'Missing' },
      { key: 'host', label: 'Host', placeholder: 'localhost', span: 'wide', section: 'Server' },
      { key: 'password', label: 'Password' },
    ])
    expect(fields.map((field) => field.key)).toEqual(['port', 'host', 'password', 'extra'])
    expect(fields[0]).toMatchObject({ label: 'TCP port', span: 'compact', type: 'int' })
    expect(fields[1]).toMatchObject({ label: 'Host', placeholder: 'localhost', required: true })
    expect(fields[3]).toMatchObject({ label: 'Extra' })
    expect(fields[3]).not.toHaveProperty('span')
  })
})

describe('field values', () => {
  const fields = resolveFields(spec, [])

  it('seeds defaults for non-secret fields only', () => {
    expect(fieldDefaults(fields)).toEqual({ host: '', port: '5432', extra: '' })
  })

  it('reports required fields that are blank', () => {
    expect(requiredFieldErrors(fields, { host: ' ', port: '5432' })).toEqual({
      host: 'Host name is required.',
    })
  })

  it('builds string params without blanks or secrets', () => {
    expect(
      paramsFromValues(fields, { host: 'db', port: '5432', extra: '', password: 'nope' }),
    ).toEqual({ host: 'db', port: '5432' })
  })

  it('prefers stored params over defaults', () => {
    expect(valuesFromParams(fields, { host: 'db', port: '6543' })).toEqual({
      host: 'db',
      port: '6543',
      extra: '',
    })
  })
})
