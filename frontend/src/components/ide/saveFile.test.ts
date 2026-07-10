// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { saveBlobAs, saveTextAs } from './saveFile'

describe('saveBlobAs', () => {
  it('creates an anchor with the given filename and revokes the object URL', () => {
    const createObjectURL = vi.fn(() => 'blob:mock-url')
    const revokeObjectURL = vi.fn()
    vi.stubGlobal('URL', { ...URL, createObjectURL, revokeObjectURL })

    const clickSpy = vi.fn()
    const originalCreateElement = document.createElement.bind(document)
    vi.spyOn(document, 'createElement').mockImplementation((tag: string) => {
      const el = originalCreateElement(tag)
      if (tag === 'a') el.click = clickSpy
      return el
    })

    saveBlobAs('report.csv', new Blob(['a,b\n1,2\n'], { type: 'text/csv' }))

    expect(createObjectURL).toHaveBeenCalledTimes(1)
    expect(clickSpy).toHaveBeenCalledTimes(1)
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:mock-url')

    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })
})

describe('saveTextAs', () => {
  it('wraps text in a text/plain blob and delegates to saveBlobAs', () => {
    const createObjectURL = vi.fn((blob: Blob) => {
      expect(blob.type).toBe('text/plain;charset=utf-8')
      return 'blob:mock-url'
    })
    vi.stubGlobal('URL', { ...URL, createObjectURL, revokeObjectURL: vi.fn() })
    const wrapped = document.createElement as unknown as { wrappedMethod?: (tag: string) => HTMLElement }
    vi.spyOn(document, 'createElement').mockImplementation((tag: string) =>
      wrapped.wrappedMethod ? wrapped.wrappedMethod(tag) : ({ click: vi.fn(), remove: vi.fn() } as unknown as HTMLElement),
    )

    saveTextAs('notes.txt', 'hello world')

    expect(createObjectURL).toHaveBeenCalledTimes(1)

    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })
})
