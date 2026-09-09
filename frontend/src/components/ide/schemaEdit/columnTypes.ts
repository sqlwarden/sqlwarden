import type { ParameterizedColumnType } from '#/lib/api/types'

export function canonicalColumnType(
  value: string,
  columnTypes: string[],
  rules: ParameterizedColumnType[] = [],
): string | null {
  const trimmed = value.trim()
  const fixed = columnTypes.find((type) => type.toUpperCase() === trimmed.toUpperCase())
  if (fixed) return fixed
  const match = /^([^()]+)\(([^()]+)\)\s*([^()]*)$/.exec(trimmed)
  if (!match) return null
  const [, name, args, suffix] = match
  const parts = args.split(',').map((part) => part.trim())
  const rule = rules.find(
    (candidate) =>
      candidate.name.toUpperCase() === name.trim().toUpperCase() &&
      (candidate.suffix ?? '').toUpperCase() === suffix.trim().replace(/\s+/g, ' ').toUpperCase() &&
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
  if (!rule) return null
  return `${rule.name}(${parts.map(Number).join(',')})${rule.suffix ? ` ${rule.suffix}` : ''}`
}
