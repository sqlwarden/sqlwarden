// Package app is the SQLWarden composition root. It builds the process from
// bootstrap configuration, owns the lifecycle of every long-running resource,
// and hands already-constructed dependencies to the web application.
//
// [Build] acquires resources in dependency order and pushes each onto a
// resource stack. A build that fails partway closes everything it already
// acquired, and [Application.Close] releases the stack in reverse order after
// stopping the process kinds.
//
// Which listeners and workers run is decided by process kinds. A [ProcessKind]
// is one runtime responsibility with its own start, readiness, and shutdown.
// Every serving process runs the "http" kind, which always serves /livez and
// /readyz and serves the API when "api" is selected, and the "runtime" kind,
// which applies instance settings changes and runs background jobs when
// "jobs" is selected. "all" selects both api and jobs. One-shot commands such
// as migrate and rotate-keys build no process kinds.
package app
