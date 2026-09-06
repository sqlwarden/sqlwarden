// Package mariadb implements the MariaDB engine: MySQL-wire-compatible, but
// with real catalog and syntax divergences MySQL doesn't have — native
// CREATE SEQUENCE objects and JSON columns that report as LONGTEXT in
// information_schema rather than a distinct JSON type. It embeds mysql.Driver
// by value and overrides SchemaSpec, InspectDirectory, InspectObjects, and
// InspectDefinition to add the "sequence" object kind, plus the column-type
// patch that restores "json" reporting for MariaDB's LONGTEXT+CHECK
// representation. Every other capability — DDL generation, classification,
// parsing, safety checking, completion, TLS/SSH tunnel support — is inherited
// unchanged via Go's method promotion, since MariaDB's SQL grammar is a
// superset of MySQL's for every statement form SQLWarden currently
// classifies.
package mariadb
