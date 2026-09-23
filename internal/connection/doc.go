// Package connection manages live target database sessions, query cursors,
// transactions, and SSH tunnels. It is the in-process session layer beneath
// execution.LocalRuntime; transports reach it only through internal/execution.
package connection
