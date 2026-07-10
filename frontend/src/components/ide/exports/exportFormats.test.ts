import { describe, expect, it } from 'vitest'
import { EXPORT_FORMATS, selectableFormats, type ExportFormat } from './exportFormats'

describe('EXPORT_FORMATS', () => {
  it('has csv enabled', () => {
    const csv = EXPORT_FORMATS.find((f) => f.value === 'csv')
    expect(csv).toBeDefined()
    expect(csv?.enabled).toBe(true)
  })
})

describe('selectableFormats', () => {
  it('excludes disabled formats', () => {
    const formats: ExportFormat[] = [
      { value: 'csv', label: 'CSV', enabled: true },
      { value: 'xlsx', label: 'Excel (XLSX)', enabled: false },
    ]
    expect(selectableFormats(formats)).toEqual([{ value: 'csv', label: 'CSV', enabled: true }])
  })

  it('defaults to EXPORT_FORMATS when called with no argument', () => {
    expect(selectableFormats()).toEqual(EXPORT_FORMATS.filter((f) => f.enabled))
  })
})
