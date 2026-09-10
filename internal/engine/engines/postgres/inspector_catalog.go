package postgres

import (
	"context"
	"database/sql"
)

// CatalogProcedures enumerates every procedure (prokind = 'p', distinct from
// CatalogFunctions' prokind = 'f') visible to the current database.
func CatalogProcedures(ctx context.Context, db *sql.DB, add func(schema, name string)) error {
	const q = `
SELECT DISTINCT n.nspname, p.proname
FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
WHERE p.prokind = 'p' AND n.nspname NOT IN ('pg_catalog', 'information_schema')
ORDER BY n.nspname, p.proname`
	return queryRefs(ctx, db, q, func(ns, name, _ string) { add(ns, name) })
}

// CatalogTriggers enumerates every user trigger (tgisinternal = false, which
// excludes triggers backing constraints like foreign keys) visible to the
// current database.
func CatalogTriggers(ctx context.Context, db *sql.DB, add func(schema, name string)) error {
	const q = `
SELECT n.nspname, t.tgname
FROM pg_trigger t
JOIN pg_class c ON c.oid = t.tgrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE NOT t.tgisinternal AND n.nspname NOT IN ('pg_catalog', 'information_schema')
ORDER BY n.nspname, t.tgname`
	return queryRefs(ctx, db, q, func(ns, name, _ string) { add(ns, name) })
}

// CatalogTypes enumerates composite, enum, and range types (typtype in
// c/e/r) visible to the current database. Domains (typtype = 'd') are a
// separate kind via CatalogDomains — the ticket treats them distinctly even
// though pg_type stores both in one catalog.
func CatalogTypes(ctx context.Context, db *sql.DB, add func(schema, name string)) error {
	const q = `
SELECT n.nspname, t.typname
FROM pg_type t JOIN pg_namespace n ON n.oid = t.typnamespace
WHERE t.typtype IN ('c', 'e', 'r')
  AND n.nspname NOT IN ('pg_catalog', 'information_schema')
  AND (t.typtype != 'c' OR EXISTS (SELECT 1 FROM pg_class c WHERE c.oid = t.typrelid AND c.relkind = 'c'))
ORDER BY n.nspname, t.typname`
	return queryRefs(ctx, db, q, func(ns, name, _ string) { add(ns, name) })
}

// CatalogDomains enumerates domains (typtype = 'd') visible to the current
// database.
func CatalogDomains(ctx context.Context, db *sql.DB, add func(schema, name string)) error {
	const q = `
SELECT n.nspname, t.typname
FROM pg_type t JOIN pg_namespace n ON n.oid = t.typnamespace
WHERE t.typtype = 'd' AND n.nspname NOT IN ('pg_catalog', 'information_schema')
ORDER BY n.nspname, t.typname`
	return queryRefs(ctx, db, q, func(ns, name, _ string) { add(ns, name) })
}

// CatalogForeignTables enumerates foreign tables visible to the current
// database.
func CatalogForeignTables(ctx context.Context, db *sql.DB, add func(schema, name string)) error {
	const q = `
SELECT n.nspname, c.relname
FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind = 'f' AND n.nspname NOT IN ('pg_catalog', 'information_schema')
ORDER BY n.nspname, c.relname`
	return queryRefs(ctx, db, q, func(ns, name, _ string) { add(ns, name) })
}
