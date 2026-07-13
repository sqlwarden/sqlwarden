import type { ObjectDetailHooks } from '../types'

// SQLite renders entirely from the base renderer: its DDL/view definition arrive
// as a "source" descriptor, and it exposes no extra table/column attributes.
export const sqliteHooks: ObjectDetailHooks = {}
