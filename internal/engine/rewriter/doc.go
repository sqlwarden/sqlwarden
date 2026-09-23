// Package rewriter defines the SQL rewriting capability: safely transforming a
// statement for a specific purpose, such as wrapping a SELECT for server-side
// pagination. An engine provides rewriting by implementing Rewriter; it is
// stateless and never touches a live connection, and it refuses to rewrite
// anything it cannot prove safe.
package rewriter
