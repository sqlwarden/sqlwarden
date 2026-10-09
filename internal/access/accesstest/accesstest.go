// Package accesstest holds assertions shared by service authorization tests.
package accesstest

import (
	"errors"
	"testing"

	"github.com/sqlwarden/internal/access"
)

// Err discards the value of a two-result service call so its error can be
// passed straight to an assertion.
func Err[T any](_ T, err error) error { return err }

// AssertForbidden fails unless err wraps access.ErrForbidden, the outcome for a
// caller who may not touch the resource's scope at all.
func AssertForbidden(t testing.TB, err error, what string) {
	t.Helper()
	AssertDenies(t, err, access.ErrForbidden, what)
}

// AssertNotFound fails unless err wraps access.ErrNotFound, the outcome for a
// resource that is missing or foreign to the caller.
func AssertNotFound(t testing.TB, err error, what string) {
	t.Helper()
	AssertDenies(t, err, access.ErrNotFound, what)
}

// AssertDenies fails unless err wraps want. Service packages alias want to
// access.ErrForbidden or access.ErrNotFound.
func AssertDenies(t testing.TB, err error, want error, what string) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("%s error = %v, want %v", what, err, want)
	}
}
