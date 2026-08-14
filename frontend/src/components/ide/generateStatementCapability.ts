import type { StatementOperation, StatementSpec } from '#/lib/api/types'

/** Fixed display order, independent of the order the backend advertises. */
export const STATEMENT_OPERATION_ORDER: StatementOperation[] = [
  'select',
  'insert',
  'update',
  'delete',
]

/** Operations advertised for a specific object kind, in display order. Empty
 *  when the driver doesn't advertise statement generation for that kind. */
export function statementOperationsFor(
  spec: StatementSpec | undefined,
  kind: string,
): StatementOperation[] {
  const advertised = spec?.objects?.find((object) => object.kind === kind)?.operations
  if (!advertised || advertised.length === 0) return []
  return STATEMENT_OPERATION_ORDER.filter((operation) => advertised.includes(operation))
}
