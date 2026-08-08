export type CsvDocument = {
  headers: string[]
  rows: string[][]
  columnCount: number
}

export class CsvParseError extends Error {
  constructor(
    message: string,
    readonly line: number,
    readonly column: number,
  ) {
    super(`${message} at line ${line}, column ${column}`)
    this.name = 'CsvParseError'
  }
}

/** Parses comma-delimited RFC 4180-style text without coercing cell values. */
export function parseCsv(source: string): CsvDocument {
  const input = source.charCodeAt(0) === 0xfeff ? source.slice(1) : source
  if (input.length === 0) return { headers: [], rows: [], columnCount: 0 }

  const records: string[][] = []
  let row: string[] = []
  let field = ''
  let inQuotes = false
  let afterQuote = false
  let line = 1
  let column = 1
  let recordHasInput = false

  const pushField = () => {
    row.push(field)
    field = ''
    afterQuote = false
  }
  const pushRecord = () => {
    pushField()
    records.push(row)
    row = []
    recordHasInput = false
  }

  for (let i = 0; i < input.length; i += 1) {
    const char = input[i]

    if (inQuotes) {
      if (char === '"') {
        if (input[i + 1] === '"') {
          field += '"'
          i += 1
          column += 2
          recordHasInput = true
          continue
        }
        inQuotes = false
        afterQuote = true
      } else {
        field += char
        if (char === '\n') {
          line += 1
          column = 1
        } else {
          column += 1
        }
      }
      recordHasInput = true
      continue
    }

    if (afterQuote) {
      if (char === ',') {
        pushField()
      } else if (char === '\n' || char === '\r') {
        pushRecord()
        if (char === '\r' && input[i + 1] === '\n') i += 1
        line += 1
        column = 0
      } else if (char !== ' ' && char !== '\t') {
        throw new CsvParseError('Unexpected character after closing quote', line, column)
      }
      column += 1
      continue
    }

    if (char === '"') {
      if (field.length > 0)
        throw new CsvParseError('Unexpected quote in unquoted field', line, column)
      inQuotes = true
      recordHasInput = true
    } else if (char === ',') {
      pushField()
      recordHasInput = true
    } else if (char === '\n' || char === '\r') {
      pushRecord()
      if (char === '\r' && input[i + 1] === '\n') i += 1
      line += 1
      column = 0
    } else {
      field += char
      recordHasInput = true
    }
    column += 1
  }

  if (inQuotes) throw new CsvParseError('Unterminated quoted field', line, column)
  if (recordHasInput || field.length > 0 || row.length > 0) pushRecord()

  const headers = records.shift() ?? []
  const columnCount = records.reduce((max, record) => Math.max(max, record.length), headers.length)
  return { headers, rows: records, columnCount }
}
