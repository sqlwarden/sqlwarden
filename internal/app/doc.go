// Package app is the SQLWarden composition root. It builds the process
// dependency graph from bootstrap configuration, owns the lifecycle of every
// long-running resource, and hands already-constructed services to the
// transports and workers that use them.
//
// Construction ([Build]) is deterministic and starts no background work.
// Background work begins only in [Application.Start] and stops in
// [Application.Close], which tears resources down in reverse construction
// order. A build that fails partway closes everything it already acquired.
//
// Which listeners and workers run is decided by process kinds. A [ProcessKind]
// is one runtime responsibility with its own start, readiness, and shutdown;
// production currently composes the single "all" process kind, which serves
// HTTP and runs every background worker in one process.
package app
