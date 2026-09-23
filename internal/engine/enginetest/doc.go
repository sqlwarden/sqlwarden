// Package enginetest provides a reusable conformance suite that every registered
// engine must satisfy. It imports testing on purpose, like net/http/httptest:
// engine packages call these from their own _test.go files.
package enginetest
