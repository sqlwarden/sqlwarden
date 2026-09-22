// Package execution defines the process-independent boundary for live target
// database sessions. HTTP and RPC adapters exchange opaque handles and typed
// requests through Runtime; only a runtime implementation owns concrete
// drivers, transactions, cursors, and their lifecycle.
package execution
