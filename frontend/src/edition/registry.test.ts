import { describe, expect, it } from 'vitest'
import { editionRegistry as community } from './community'
import { editionRegistry as enterprise } from '../enterprise'
import { validateEditionRegistry } from './types'

describe('edition registries', () => {
  it.each([
    ['community', community],
    ['enterprise', enterprise],
  ])('%s starts with no placeholder contributions', (_name, registry) => {
    expect(registry).toEqual({ routes: [], navigation: [], settings: [], gates: [] })
    expect(validateEditionRegistry(registry)).toBe(registry)
  })
})
