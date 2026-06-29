import type { DriverHooks } from '../registry'

// SQLite renders entirely from the base renderer: its DDL/view definition arrive
// as a "source" descriptor, and it exposes no extra table/column attributes.
export const sqliteHooks: DriverHooks = {}
