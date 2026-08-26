import { describe, it, expect, vi, afterEach } from 'vitest'
import {
  columnList,
  qualifiedColumn,
  readClipboardFallback,
  rowToTsv,
  rowToJson,
  selectionToTsv,
  valuesToLines,
} from './clipboard'
import { dialectFor } from '../sqlDialect'

describe('columnList', () => {
  it('joins names with comma+space', () => {
    expect(columnList(['id', 'name', 'email'])).toBe('id, name, email')
  })
  it('returns empty string for no columns', () => {
    expect(columnList([])).toBe('')
  })
})

describe('qualifiedColumn', () => {
  const pg = dialectFor('postgres')
  it('joins bare identifiers with a dot, unquoted', () => {
    expect(qualifiedColumn(pg, 'users', 'id')).toBe('users.id')
  })
  it('quotes identifiers that need quoting', () => {
    expect(qualifiedColumn(pg, 'Order', 'Id')).toBe('"Order"."Id"')
  })
})

describe('result-grid producers', () => {
  it('rowToTsv joins cells with tabs', () => {
    expect(rowToTsv(['1', 'alice', 'NULL'])).toBe('1\talice\tNULL')
  })
  it('rowToJson keys cells by column name', () => {
    expect(rowToJson(['id', 'name'], ['1', 'alice'])).toBe('{\n  "id": "1",\n  "name": "alice"\n}')
  })
  it('selectionToTsv joins rows with newlines', () => {
    expect(
      selectionToTsv([
        ['1', 'a'],
        ['2', 'b'],
      ]),
    ).toBe('1\ta\n2\tb')
  })
  it('valuesToLines joins values with newlines', () => {
    expect(valuesToLines(['a', 'b', 'c'])).toBe('a\nb\nc')
  })
})

describe('readClipboardFallback', () => {
  // jsdom doesn't implement execCommand at all, so stub it directly rather
  // than spying on a nonexistent method.
  afterEach(() => {
    // @ts-expect-error test-only cleanup of the stub
    delete document.execCommand
  })

  it('returns the pasted text when execCommand succeeds', () => {
    document.execCommand = vi.fn((command: string) => {
      if (command !== 'paste') return false
      const el = document.activeElement as HTMLTextAreaElement
      el.value = 'select 1'
      return true
    })
    expect(readClipboardFallback()).toBe('select 1')
  })

  it('returns null when execCommand is unsupported', () => {
    document.execCommand = vi.fn(() => {
      throw new Error('not supported')
    })
    expect(readClipboardFallback()).toBeNull()
  })

  it('returns null when the clipboard is empty', () => {
    document.execCommand = vi.fn(() => true)
    expect(readClipboardFallback()).toBeNull()
  })
})
