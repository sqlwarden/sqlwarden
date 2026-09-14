import { describe, it, expect } from 'vitest'
import { findHoveredStatement } from './statementHover'
import { sqlStatementsWithOffsets } from './sqlStatements'

describe('findHoveredStatement', () => {
  const text = 'select 1;\n\nselect 2;'
  const statements = sqlStatementsWithOffsets(text)

  it('returns null when offset is null', () => {
    expect(findHoveredStatement(statements, null)).toBeNull()
  })

  it('returns the statement containing the offset', () => {
    const offset = text.indexOf('select 2')
    expect(findHoveredStatement(statements, offset)).toEqual(statements[1])
  })

  it('returns the statement when offset is exactly at its start', () => {
    expect(findHoveredStatement(statements, statements[0].start)).toEqual(statements[0])
  })

  it('returns null when offset sits in the gap between statements', () => {
    const gapOffset = text.indexOf('\n\n') + 1
    expect(findHoveredStatement(statements, gapOffset)).toBeNull()
  })

  it('returns the statement when offset is exactly at its end', () => {
    // This is where the mouse resolves when hovering the inline Run/Explain
    // hint, which is positioned right after the statement's last character —
    // it must stay matched or the hint disappears as the pointer approaches it.
    expect(findHoveredStatement(statements, statements[1].end)).toEqual(statements[1])
  })

  it('returns null when offset is past the end of the last statement', () => {
    const trailingText = `${text}\n`
    const trailingStatements = sqlStatementsWithOffsets(trailingText)
    expect(findHoveredStatement(trailingStatements, trailingText.length)).toBeNull()
  })

  it('returns null for an empty statement list', () => {
    expect(findHoveredStatement([], 0)).toBeNull()
  })
})
