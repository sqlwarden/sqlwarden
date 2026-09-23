// Package credentials contains local-execution implementations of
// execution.CredentialProvider. The core provider resolves and decrypts the
// encrypted credential columns stored in the SQLWarden metadata database
// without exposing secret material in errors. It runs with LocalRuntime in an
// all-in-one process or in the connector of a split deployment.
package credentials
