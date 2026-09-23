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
// is one runtime responsibility with its own start, readiness, and shutdown.
// Transports supply the concrete kinds through [Options.ProcessKinds]; the
// supported kinds are "all" (the public HTTP API with in-process target
// execution and every background worker), "api" (the public HTTP API,
// delegating target execution to a connector), and "connector" (the internal
// execution endpoint that owns live target sessions and resolves credentials).
package app
