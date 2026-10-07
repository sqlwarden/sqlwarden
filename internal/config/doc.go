// Package config owns SQLWarden bootstrap configuration: the deployment-managed
// inputs a process needs before it can open its application database.
//
// Configuration is grouped into three categories:
//
//   - Bootstrap: listen address, process kinds, database driver and DSN
//     reference, migration policy, TLS, file-storage topology.
//   - Runtime: database-backed settings administrators change without a restart.
//     Runtime settings are owned by the application database, not this package;
//     they appear here only as vocabulary (for example log levels).
//   - Secrets: encryption keys, signing keys, and credentials.
//
// Values load from CLI flags, environment variables, mounted secret files, and
// configuration files. See [Load] for the precedence rules and [Diagnostic] for
// the redacted effective-configuration report.
package config
