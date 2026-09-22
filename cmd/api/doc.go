// Command api starts the SQLWarden HTTP API server.
//
// Without a subcommand it builds and runs the process kinds named by
// process_kinds and serves until it receives SIGINT or SIGTERM, after which it
// shuts down gracefully within shutdown_timeout.
//
// Subcommands:
//
//	migrate       apply database migrations under the migration lock and exit
//	rotate-keys   re-encrypt application-encrypted data with the primary key
package main
