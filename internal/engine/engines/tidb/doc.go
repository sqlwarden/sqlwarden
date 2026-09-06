// Package tidb implements the TiDB engine per the extension pattern
// documented in mysql/doc.go: driver embeds mysql.Driver by value, and Go's
// method promotion satisfies engine.Driver and every optional capability
// interface through the embedded type automatically. Only the methods that
// genuinely diverge are overridden here:
//
//   - Dialect reports engine.DialectMySQL explicitly (matching what
//     mysql.Driver already returns): TiDB targets the MySQL SQL surface for
//     every statement form SQLWarden parses, classifies, or completes, so it
//     needs no dialect identity of its own.
//   - SchemaSpec/InspectDirectory/InspectObjects/InspectDefinition drop the
//     function, procedure, and trigger kinds — TiDB has no stored routine or
//     trigger support — and add TiDB's native sequence object kind
//     (CatalogSequences, SequenceObjects in catalog.go), the same catalog
//     divergence MariaDB has from plain MySQL.
//
// Everything else — connection handling, TLS, SSH tunneling, DDL, parsing,
// classification, safety checking, completion — is inherited unchanged via
// Go's method promotion, since TiDB implements the MySQL wire protocol and
// enough of information_schema to satisfy those queries as-is.
package tidb
