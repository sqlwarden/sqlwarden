# SQL completion core

This package is the semantic completion boundary for PostgreSQL and MySQL. Its
design and completion scenarios are adapted from Bytebase's MIT-licensed
implementation; see `PROVENANCE.md` and `LICENSE.bytebase`.

## Ownership boundary

Omni owns:

- dialect tokenization and grammar candidates;
- strict SQL parsing;
- PostgreSQL completion scope snapshots;
- built-in dialect vocabulary and native in-memory catalogs.

SQLWarden owns:

- canonical engine metadata and its reusable immutable `metadata.Index`;
- the thin `SchemaResolver` adapter from schema objects to completion relations;
- candidate types and mapping to the editor API;
- exact alias/qualifier resolution;
- CTE and derived-relation projected columns;
- DML target/reference resolution;
- ambiguity handling and identifier quoting at the engine boundary;
- cache invalidation by connection and schema snapshot version.

MySQL reference collection is deliberately confined to `mysql/complete.go`.
When Omni exports a MySQL scope snapshot equivalent to PostgreSQL's
`CollectCompletion`, that collector can be replaced without changing callers.

## Test framework

`completiontest.Run` is a black-box, dialect-neutral scenario harness. A
scenario contains one `|` caret marker plus exact required and excluded
candidates. Assertions intentionally ignore unrelated grammar candidates, so
upgrading Omni can add valid keywords without rewriting semantic fixtures.

The dialect suites cover:

- aliases, qualified references, and unknown qualifiers;
- ambiguous columns across joins;
- quoted aliases and partial prefixes;
- CTEs, explicit CTE columns, and inferred projected columns;
- derived tables and correlated subqueries;
- statement isolation and incomplete trailing sheets;
- SELECT-list completion before `FROM`;
- INSERT, UPDATE, DELETE, joined updates, `VALUES`, and `INSERT SELECT`;
- CTE-backed DML;
- bounded scaling on large multi-statement editor buffers.

Engine tests additionally cover candidate-to-API mapping, identifier quoting,
schema object kinds, lexical vocabulary, cancellation, invalid cursors, and
automatic bare-`SELECT` curation. Omni's own parser and completion suites are
also part of the dependency-upgrade verification gate.
