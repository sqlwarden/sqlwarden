// Package execution defines the process-independent boundary for live target
// database sessions. HTTP and RPC adapters exchange opaque handles and typed
// requests through Runtime; only a runtime implementation owns concrete
// drivers, transactions, cursors, and their lifecycle.
//
// LocalRuntime executes in-process on top of internal/connection. WorkerRuntime
// forwards the same operations to a connector process over the internal HTTP
// protocol, locating session owners through a SessionDirectory. Stored target
// credentials needed at execution time are resolved on the connector side
// through a CredentialProvider; an API-only process constructs no provider and
// the internal protocol carries connection identifiers, never credentials.
package execution
