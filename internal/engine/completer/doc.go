// Package completer defines the SQL completion capability: cursor-aware
// suggestions for an in-progress statement. An engine provides completion by
// implementing Completer; it is stateless and never touches a live connection —
// any schema context it needs is passed in as a catalog by the caller.
package completer
