import { describe, it, expect } from 'vitest'
import { sqlStatementAtCursor } from './IdeToolbar'

// Helper: find the index of the Nth occurrence of a substring.
function nthIndex(text: string, sub: string, n: number): number {
  let i = -1
  for (let k = 0; k < n; k++) {
    i = text.indexOf(sub, i + 1)
    if (i === -1) throw new Error(`"${sub}" not found (occurrence ${k + 1})`)
  }
  return i
}

describe('sqlStatementAtCursor', () => {
  // ─── Basic cursor-position behaviour (the reported bug) ──────────────────────

  describe('cursor after first semicolon (reported bug)', () => {
    const text = 'select 1;\n\nselect 2;'
    const semi1 = text.indexOf(';') // index 8

    it('cursor ON the semicolon → select 1', () => {
      expect(sqlStatementAtCursor(text, semi1)).toBe('select 1;')
    })

    it('cursor immediately after semicolon → select 1', () => {
      expect(sqlStatementAtCursor(text, semi1 + 1)).toBe('select 1;')
    })

    it('cursor in empty line between statements → select 1', () => {
      expect(sqlStatementAtCursor(text, semi1 + 2)).toBe('select 1;')
    })

    it('cursor at start of second statement → select 2', () => {
      const s2 = text.indexOf('select 2')
      expect(sqlStatementAtCursor(text, s2)).toBe('select 2;')
    })

    it('cursor inside second statement → select 2', () => {
      const s2 = text.indexOf('select 2')
      expect(sqlStatementAtCursor(text, s2 + 4)).toBe('select 2;')
    })

    it('cursor on second semicolon → select 2', () => {
      const semi2 = text.lastIndexOf(';')
      expect(sqlStatementAtCursor(text, semi2)).toBe('select 2;')
    })
  })

  // ─── No semicolons ────────────────────────────────────────────────────────────

  describe('no semicolons', () => {
    it('single statement, cursor anywhere → full text trimmed', () => {
      const text = 'SELECT * FROM users'
      expect(sqlStatementAtCursor(text, 0)).toBe('SELECT * FROM users')
      expect(sqlStatementAtCursor(text, 10)).toBe('SELECT * FROM users')
      expect(sqlStatementAtCursor(text, text.length - 1)).toBe('SELECT * FROM users')
    })

    it('multiline statement without semicolon → full text trimmed', () => {
      const text = 'SELECT id,\n  name\nFROM accounts\nWHERE id = 1'
      expect(sqlStatementAtCursor(text, 0)).toBe(text.trim())
      expect(sqlStatementAtCursor(text, 20)).toBe(text.trim())
    })

    it('leading/trailing whitespace stripped when no semicolons', () => {
      const text = '  SELECT 1  '
      expect(sqlStatementAtCursor(text, 2)).toBe('SELECT 1')
    })
  })

  // ─── Incomplete queries (no terminating semicolon) ────────────────────────────

  describe('incomplete queries', () => {
    it('second statement has no semicolon → cursor in it returns its text', () => {
      const text = 'SELECT 1;\nSELECT'
      const s2 = text.indexOf('SELECT', 1)
      expect(sqlStatementAtCursor(text, s2)).toBe('SELECT')
    })

    it('cursor before any semicolon in mixed text → first statement', () => {
      const text = 'SELECT 1;\nSELECT 2'
      expect(sqlStatementAtCursor(text, 3)).toBe('SELECT 1;')
    })

    it('incomplete multiline second statement', () => {
      const text = 'INSERT INTO t VALUES (1);\nUPDATE t SET'
      const s2 = text.indexOf('UPDATE')
      expect(sqlStatementAtCursor(text, s2 + 5)).toBe('UPDATE t SET')
    })
  })

  // ─── Special characters inside string literals ────────────────────────────────

  describe('special characters in string literals', () => {
    it('semicolon inside single-quoted string is not a boundary', () => {
      const text = "SELECT 'hello; world';\nSELECT 2;"
      // cursor inside the string literal → first statement
      const inStr = text.indexOf('hello')
      expect(sqlStatementAtCursor(text, inStr)).toBe("SELECT 'hello; world';")
    })

    it('escaped single-quote inside string (two single quotes) does not close string', () => {
      const text = "SELECT 'it''s a test; not a semi';\nSELECT 2;"
      const inStr = text.indexOf("it''s")
      expect(sqlStatementAtCursor(text, inStr)).toBe("SELECT 'it''s a test; not a semi';")
    })

    it('semicolon inside double-quoted identifier is not a boundary', () => {
      const text = 'SELECT "my;col" FROM t;\nSELECT 2;'
      const inId = text.indexOf('my;col')
      expect(sqlStatementAtCursor(text, inId)).toBe('SELECT "my;col" FROM t;')
    })

    it('escaped double-quote inside identifier does not close it', () => {
      const text = 'SELECT "col""name" FROM t;\nSELECT 2;'
      const inId = text.indexOf('col""name')
      expect(sqlStatementAtCursor(text, inId)).toBe('SELECT "col""name" FROM t;')
    })

    it('newline and tab inside string literal', () => {
      const text = "SELECT 'line1\n\tline2';\nSELECT 2;"
      const s2 = text.lastIndexOf('SELECT 2')
      expect(sqlStatementAtCursor(text, s2)).toBe('SELECT 2;')
      const s1 = text.indexOf("SELECT '")
      expect(sqlStatementAtCursor(text, s1)).toBe("SELECT 'line1\n\tline2';")
    })

    it('unicode characters in string literal', () => {
      const text = "SELECT '日本語; test';\nSELECT 2;"
      const inStr = text.indexOf('日本語')
      expect(sqlStatementAtCursor(text, inStr)).toBe("SELECT '日本語; test';")
    })
  })

  // ─── Comments ─────────────────────────────────────────────────────────────────

  describe('comments', () => {
    it('semicolon inside line comment does not create a new statement boundary', () => {
      // The `;` in `end;` is in a line comment — it must NOT be treated as a
      // statement separator.  Only two statements exist: SELECT 1 and SELECT 2.
      // A cursor positioned in SELECT 2 must return SELECT 2 (not some third
      // fragment created by the in-comment semicolon).
      const text = 'SELECT 1; -- end; of statement\nSELECT 2;'
      const s2 = text.indexOf('SELECT 2')
      // Cursor in SELECT 2 → should return the SELECT 2 span, not a spurious
      // fragment caused by the comment semicolon.
      expect(sqlStatementAtCursor(text, s2)).toContain('SELECT 2')
      // Cursor in SELECT 1 (before the real semicolon) → SELECT 1.
      const s1 = text.indexOf('SELECT 1')
      expect(sqlStatementAtCursor(text, s1)).toBe('SELECT 1;')
    })

    it('semicolon inside block comment is not a boundary', () => {
      const text = 'SELECT 1 /* semi; here */;\nSELECT 2;'
      const inComment = text.indexOf('semi;')
      expect(sqlStatementAtCursor(text, inComment)).toBe('SELECT 1 /* semi; here */;')
    })

    it('leading line comment is grouped with its statement in the same span', () => {
      // A comment before a statement has no separating semicolon, so it belongs
      // to the same span.  The important property is that cursor in SELECT 1
      // does NOT return SELECT 2.
      const text = '-- get user\nSELECT 1;\nSELECT 2;'
      const s1 = text.indexOf('SELECT 1')
      const result = sqlStatementAtCursor(text, s1)
      expect(result).toContain('SELECT 1')
      expect(result).not.toContain('SELECT 2')
    })
  })

  // ─── Long queries ──────────────────────────────────────────────────────────────

  describe('long queries', () => {
    it('multiline first statement, cursor on its last line', () => {
      const text = [
        'SELECT',
        '  id,',
        '  name,',
        '  email',
        'FROM accounts',
        'WHERE active = true',
        'ORDER BY name;',
        '',
        'SELECT 2;',
      ].join('\n')
      const orderBy = text.indexOf('ORDER BY')
      expect(sqlStatementAtCursor(text, orderBy)).toContain('SELECT')
      expect(sqlStatementAtCursor(text, orderBy)).toContain('ORDER BY name;')
      expect(sqlStatementAtCursor(text, orderBy)).not.toContain('SELECT 2')
    })

    it('cursor in blank line between long statements → preceding statement', () => {
      const text = [
        'SELECT id FROM accounts;',
        '',
        'SELECT name FROM accounts;',
      ].join('\n')
      const blankLine = text.indexOf('\n') + 1 // first blank line char
      expect(sqlStatementAtCursor(text, blankLine)).toBe('SELECT id FROM accounts;')
    })
  })

  // ─── Three-statement sequences ────────────────────────────────────────────────

  describe('three statements', () => {
    const text = 'SELECT 1;\nSELECT 2;\nSELECT 3;'
    const s1 = text.indexOf('SELECT 1')
    const semi1 = text.indexOf(';')
    const s2 = text.indexOf('SELECT 2')
    const semi2 = nthIndex(text, ';', 2)
    const s3 = text.indexOf('SELECT 3')

    it('cursor in first statement', () => {
      expect(sqlStatementAtCursor(text, s1 + 3)).toBe('SELECT 1;')
    })

    it('cursor in whitespace between 1 and 2 → SELECT 1', () => {
      expect(sqlStatementAtCursor(text, semi1 + 1)).toBe('SELECT 1;')
    })

    it('cursor in second statement', () => {
      expect(sqlStatementAtCursor(text, s2 + 3)).toBe('SELECT 2;')
    })

    it('cursor on second semicolon', () => {
      expect(sqlStatementAtCursor(text, semi2)).toBe('SELECT 2;')
    })

    it('cursor in whitespace between 2 and 3 → SELECT 2', () => {
      expect(sqlStatementAtCursor(text, semi2 + 1)).toBe('SELECT 2;')
    })

    it('cursor in third statement', () => {
      expect(sqlStatementAtCursor(text, s3 + 3)).toBe('SELECT 3;')
    })
  })

  // ─── Edge: cursor at boundaries ───────────────────────────────────────────────

  describe('boundary positions', () => {
    it('cursor at position 0', () => {
      expect(sqlStatementAtCursor('SELECT 1;\nSELECT 2;', 0)).toBe('SELECT 1;')
    })

    it('cursor past end of text → last statement', () => {
      const text = 'SELECT 1;\nSELECT 2;'
      // Position beyond text.length falls in no span, fallback picks last.
      expect(sqlStatementAtCursor(text, text.length)).toBe('SELECT 2;')
    })

    it('single query with trailing semicolon, cursor at end', () => {
      const text = 'SELECT 1;'
      expect(sqlStatementAtCursor(text, text.length - 1)).toBe('SELECT 1;')
      expect(sqlStatementAtCursor(text, text.length)).toBe('SELECT 1;')
    })

    it('empty text → empty string', () => {
      expect(sqlStatementAtCursor('', 0)).toBe('')
    })

    it('only whitespace → empty string', () => {
      expect(sqlStatementAtCursor('   \n\n   ', 2)).toBe('')
    })
  })
})
