import { renderHook } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { BrandFullLogo } from './BrandFullLogo'
import { BrandMark } from './BrandMark'
import { defaultBrand, useBrand } from './brand'

describe('useBrand', () => {
  it('returns the default SQLWarden brand config', () => {
    const { result } = renderHook(() => useBrand())
    expect(result.current).toBe(defaultBrand)
    expect(result.current.productName).toBe('SQLWarden')
    expect(result.current.LogoMark).toBe(BrandMark)
    expect(result.current.LogoLockup).toBe(BrandFullLogo)
    expect(result.current.faviconPath).toBe('/favicon.ico')
    expect(result.current.manifestPath).toBe('/manifest.json')
  })
})
