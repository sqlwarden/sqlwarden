import { describe, expect, it } from 'vitest'
import {
  emptySecretStates,
  reduceSecret,
  secretStatesFromViews,
  secretsPayload,
  type SecretFieldState,
} from './secretField'

const saved: SecretFieldState = { kind: 'saved', revealable: false }
const revealable: SecretFieldState = { kind: 'saved', revealable: true }

describe('secretStatesFromViews', () => {
  it('starts every secret empty without a view', () => {
    expect(secretStatesFromViews(undefined)).toEqual(emptySecretStates())
  })

  it('maps stored secrets to saved and carries revealable', () => {
    const states = secretStatesFromViews({
      password: { set: true, source: 'stored', revealable: true },
      ssh_password: { set: true, source: 'stored', revealable: false },
      ssh_passphrase: { set: false, revealable: false },
    })
    expect(states.password).toEqual({ kind: 'saved', revealable: true })
    expect(states.ssh_password).toEqual({ kind: 'saved', revealable: false })
    expect(states.ssh_passphrase).toEqual({ kind: 'empty' })
    expect(states.tls_client_key).toEqual({ kind: 'empty' })
  })

  it('maps externally managed secrets to managed and never revealable', () => {
    const states = secretStatesFromViews({
      password: { set: true, source: 'reference', revealable: true },
    })
    expect(states.password).toEqual({ kind: 'managed', source: 'reference' })
  })
})

describe('reduceSecret', () => {
  it('empty becomes replaced when typed into and returns when erased', () => {
    const typed = reduceSecret({ kind: 'empty' }, { type: 'edit', value: 'abc' })
    expect(typed).toEqual({ kind: 'replaced', value: 'abc', previous: { kind: 'empty' } })
    expect(reduceSecret(typed, { type: 'edit', value: '' })).toEqual({ kind: 'empty' })
  })

  it('saved becomes replaced when typed into and returns to saved when erased', () => {
    const typed = reduceSecret(revealable, { type: 'edit', value: 'new' })
    expect(typed).toEqual({ kind: 'replaced', value: 'new', previous: revealable })
    expect(reduceSecret(typed, { type: 'edit', value: '' })).toEqual(revealable)
  })

  it('keeps the original resting state across repeated edits', () => {
    const first = reduceSecret(saved, { type: 'edit', value: 'a' })
    const second = reduceSecret(first, { type: 'edit', value: 'ab' })
    expect(second).toEqual({ kind: 'replaced', value: 'ab', previous: saved })
  })

  it('clears a saved secret and restores it', () => {
    const cleared = reduceSecret(saved, { type: 'clear' })
    expect(cleared).toEqual({ kind: 'cleared', previous: saved })
    expect(reduceSecret(cleared, { type: 'restore' })).toEqual(saved)
  })

  it('clearing a replaced saved secret removes the stored value, not just the edit', () => {
    const replaced = reduceSecret(saved, { type: 'edit', value: 'new' })
    expect(reduceSecret(replaced, { type: 'clear' })).toEqual({ kind: 'cleared', previous: saved })
  })

  it('clearing a secret that was never stored leaves it empty', () => {
    expect(reduceSecret({ kind: 'empty' }, { type: 'clear' })).toEqual({ kind: 'empty' })
    const typed = reduceSecret({ kind: 'empty' }, { type: 'edit', value: 'x' })
    expect(reduceSecret(typed, { type: 'clear' })).toEqual({ kind: 'empty' })
  })

  it('restore discards a replacement', () => {
    const replaced = reduceSecret(saved, { type: 'edit', value: 'new' })
    expect(reduceSecret(replaced, { type: 'restore' })).toEqual(saved)
  })

  it('ignores typing into a cleared secret until it is restored', () => {
    const cleared = reduceSecret(saved, { type: 'clear' })
    expect(reduceSecret(cleared, { type: 'edit', value: 'x' })).toEqual(cleared)
  })

  it('never changes a managed secret', () => {
    const managed: SecretFieldState = { kind: 'managed', source: 'reference' }
    expect(reduceSecret(managed, { type: 'edit', value: 'x' })).toEqual(managed)
    expect(reduceSecret(managed, { type: 'clear' })).toEqual(managed)
    expect(reduceSecret(managed, { type: 'restore' })).toEqual(managed)
  })
})

describe('secretsPayload', () => {
  it('omits untouched secrets so the server keeps them', () => {
    const states = {
      ...emptySecretStates(),
      password: saved,
      ssh_password: { kind: 'managed', source: 'reference' } as const,
    }
    expect(secretsPayload(states)).toEqual({})
  })

  it('sends a string to replace and null to clear', () => {
    const states = {
      ...emptySecretStates(),
      password: { kind: 'replaced', value: 'hunter2', previous: saved } as const,
      ssh_private_key: { kind: 'cleared', previous: saved } as const,
      tls_client_key: { kind: 'replaced', value: 'KEY', previous: { kind: 'empty' } } as const,
    }
    expect(secretsPayload(states)).toEqual({
      password: 'hunter2',
      ssh_private_key: null,
      tls_client_key: 'KEY',
    })
  })
})
