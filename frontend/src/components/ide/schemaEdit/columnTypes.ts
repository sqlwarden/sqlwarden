import type { ParameterizedColumnType } from '#/lib/api/types'

// Mirrors internal/engine/ddl.customColumnTypePattern: bare or
// schema-qualified names, multi-word suffixes, one parenthesized argument
// list, trailing array brackets. Kept in sync so the UI's notion of "looks
// like a type" matches what the backend will actually accept.
const CUSTOM_COLUMN_TYPE_PATTERN =
  /^[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)?(?: [A-Za-z_][A-Za-z0-9_]*)*(?:\([A-Za-z0-9_, ]+\))?(?:\[\])*$/
const MAX_CUSTOM_COLUMN_TYPE_LENGTH = 128

export function validCustomColumnTypeSyntax(value: string): boolean {
  return (
    value.length > 0 &&
    value.length <= MAX_CUSTOM_COLUMN_TYPE_LENGTH &&
    CUSTOM_COLUMN_TYPE_PATTERN.test(value)
  )
}

function leadingTypeName(value: string): string {
  const parenIndex = value.indexOf('(')
  const spaceIndex = value.indexOf(' ')
  const cut = [parenIndex, spaceIndex].filter((i) => i > 0).sort((a, b) => a - b)[0]
  return (cut === undefined ? value : value.slice(0, cut)).trim()
}

export function canonicalColumnType(
  value: string,
  columnTypes: string[],
  rules: ParameterizedColumnType[] = [],
  allowCustomTypes = false,
): string | null {
  const trimmed = value.trim()
  const fixed = columnTypes.find((type) => type.toUpperCase() === trimmed.toUpperCase())
  if (fixed) return fixed
  const match = /^([^()]+)\(([^()]+)\)\s*([^()]*)$/.exec(trimmed)
  if (match) {
    const [, name, args, suffix] = match
    const parts = args.split(',').map((part) => part.trim())
    const rule = rules.find(
      (candidate) =>
        candidate.name.toUpperCase() === name.trim().toUpperCase() &&
        (candidate.suffix ?? '').toUpperCase() ===
          suffix.trim().replace(/\s+/g, ' ').toUpperCase() &&
        parts.length <= candidate.parameters.length &&
        candidate.parameters.every((parameter, i) => {
          if (i >= parts.length) return parameter.optional
          return (
            /^[+-]?\d+$/.test(parts[i]) &&
            Number(parts[i]) >= parameter.min &&
            Number(parts[i]) <= parameter.max
          )
        }),
    )
    if (rule)
      return `${rule.name}(${parts.map(Number).join(',')})${rule.suffix ? ` ${rule.suffix}` : ''}`
    // A recognized type name with out-of-range arguments is a malformed
    // known type, not an unknown extension type — never fall through.
    if (rules.some((candidate) => candidate.name.toUpperCase() === name.trim().toUpperCase()))
      return null
  }
  const knownName = leadingTypeName(trimmed).toUpperCase()
  const isKnownName =
    rules.some((rule) => rule.name.toUpperCase() === knownName) ||
    columnTypes.some((type) => leadingTypeName(type).toUpperCase() === knownName)
  if (allowCustomTypes && !isKnownName && validCustomColumnTypeSyntax(trimmed)) return trimmed
  return null
}
