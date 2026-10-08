// Package executiontest holds the behavior every execution.Runtime must
// satisfy, whether it runs in-process or behind a transport. The suite issues
// SQLite-compatible SQL, so the database it runs against must accept it.
package executiontest
