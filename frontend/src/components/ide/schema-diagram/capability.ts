import type { SchemaSpec } from '#/lib/api/types'

/** True when the connection exposes at least one diagram-capable object kind. */
export function diagramSupported(spec: SchemaSpec | undefined): boolean {
  return Boolean(spec?.kinds?.some((k) => k.supports_diagram))
}

/** True when a specific object kind supports a per-object diagram. */
export function diagramSupportedForKind(spec: SchemaSpec | undefined, kind: string): boolean {
  return Boolean(spec?.kinds?.some((k) => k.kind === kind && k.supports_diagram))
}
