import { describe, expect, it } from 'vitest'
import type { EditorTab } from '../useIdeStore'
import { isCsvFileTab, isCsvFileTooLarge, MAX_BROWSER_CSV_BYTES } from './csvFile'

function fileTab(overrides: Partial<EditorTab> = {}): EditorTab {
  return {
    id: 'file:1',
    workspaceId: 1,
    title: 'query.sql',
    kind: 'file',
    fileId: 1,
    content: '',
    ...overrides,
  }
}

describe('isCsvFileTab', () => {
  it('detects CSV media types including charset parameters', () => {
    expect(isCsvFileTab(fileTab({ fileMediaType: 'text/csv; charset=utf-8' }))).toBe(true)
  })

  it('falls back to a case-insensitive filename extension for persisted tabs', () => {
    expect(isCsvFileTab(fileTab({ title: 'REPORT.CSV' }))).toBe(true)
  })

  it('recognizes an explicit CSV file kind', () => {
    expect(isCsvFileTab(fileTab({ fileKind: 'CSV' }))).toBe(true)
  })

  it('does not route non-file or non-CSV tabs to the viewer', () => {
    expect(isCsvFileTab(fileTab())).toBe(false)
    expect(isCsvFileTab(fileTab({ kind: 'scratch', title: 'report.csv' }))).toBe(false)
  })
})

describe('isCsvFileTooLarge', () => {
  it('limits only CSV files larger than the browser preview ceiling', () => {
    expect(
      isCsvFileTooLarge(fileTab({ title: 'report.csv', fileSizeBytes: MAX_BROWSER_CSV_BYTES + 1 })),
    ).toBe(true)
    expect(
      isCsvFileTooLarge(fileTab({ title: 'report.csv', fileSizeBytes: MAX_BROWSER_CSV_BYTES })),
    ).toBe(false)
    expect(
      isCsvFileTooLarge(fileTab({ title: 'query.sql', fileSizeBytes: MAX_BROWSER_CSV_BYTES + 1 })),
    ).toBe(false)
  })

  it('allows CSV tabs without size metadata to load', () => {
    expect(isCsvFileTooLarge(fileTab({ title: 'persisted.csv' }))).toBe(false)
  })
})
