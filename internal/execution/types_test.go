package execution

import (
	"errors"
	"fmt"
	"testing"
)

func TestScopeValid(t *testing.T) {
	full := Scope{OrgID: "o", WorkspaceID: "w", AccountID: "a", ConnectionID: "c"}
	if !full.Valid() {
		t.Fatal("full scope must be valid")
	}
	if (Scope{}).Valid() {
		t.Fatal("zero scope must be invalid")
	}
	cases := map[string]func(*Scope){
		"org":        func(s *Scope) { s.OrgID = "" },
		"workspace":  func(s *Scope) { s.WorkspaceID = "" },
		"account":    func(s *Scope) { s.AccountID = "" },
		"connection": func(s *Scope) { s.ConnectionID = "" },
	}
	for name, clear := range cases {
		s := full
		clear(&s)
		if s.Valid() {
			t.Errorf("scope missing %s must be invalid", name)
		}
	}
}

func TestFailure(t *testing.T) {
	var err error = fmt.Errorf("wrapped: %w", &Failure{Code: FailureCursorLost, Retryable: true, Err: ErrCursorLost})
	var f *Failure
	if !errors.As(err, &f) || f.Code != FailureCursorLost || !f.Retryable {
		t.Fatalf("errors.As failed: %#v", f)
	}
	if !errors.Is(err, ErrCursorLost) {
		t.Fatal("errors.Is must reach the underlying error")
	}
	if got := (&Failure{Code: FailureSessionNotFound}).Error(); got != "session_not_found" {
		t.Fatalf("Error() = %q", got)
	}
}
