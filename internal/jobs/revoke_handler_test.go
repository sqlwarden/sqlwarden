package jobs

import (
	"context"
	"errors"
	"testing"
)

type fakeSessionRevoker struct {
	err   error
	calls []string
}

func (f *fakeSessionRevoker) RevokeConnection(_ context.Context, id string) (int, error) {
	f.calls = append(f.calls, "connection:"+id)
	return 3, f.err
}

func (f *fakeSessionRevoker) RevokeWorkspaceAccount(_ context.Context, ws, acct string) (int, error) {
	f.calls = append(f.calls, "workspace_account:"+ws+":"+acct)
	return 3, f.err
}

func (f *fakeSessionRevoker) RevokeOrgAccount(_ context.Context, org, acct string) (int, error) {
	f.calls = append(f.calls, "org_account:"+org+":"+acct)
	return 3, f.err
}

func revokeRuntime(input string) Runtime {
	return Runtime{Job: Record{InputJSON: input}}
}

func TestSessionRevokeHandlerRevokesEachKind(t *testing.T) {
	tests := []struct {
		input string
		call  string
	}{
		{`{"kind":"connection","connection_id":"4"}`, "connection:4"},
		{`{"kind":"workspace_account","workspace_id":"7","account_id":"9"}`, "workspace_account:7:9"},
		{`{"kind":"org_account","org_id":"2","account_id":"9"}`, "org_account:2:9"},
	}
	for _, tc := range tests {
		fake := &fakeSessionRevoker{}
		out, err := SessionRevokeHandler(fake).Handle(context.Background(), revokeRuntime(tc.input))
		if err != nil {
			t.Fatalf("%s: unexpected error %v", tc.call, err)
		}
		if out.(map[string]any)["revoked"] != 3 {
			t.Fatalf("%s: output = %v", tc.call, out)
		}
		if len(fake.calls) != 1 || fake.calls[0] != tc.call {
			t.Fatalf("calls = %v, want [%s]", fake.calls, tc.call)
		}
	}
}

func TestSessionRevokeHandlerFailures(t *testing.T) {
	tests := []struct {
		name      string
		revoker   SessionRevoker
		input     string
		code      string
		retryable bool
	}{
		{"unknown kind", &fakeSessionRevoker{}, `{"kind":"nope"}`, "invalid_session_revoke_input", false},
		{"invalid json", &fakeSessionRevoker{}, `{`, "invalid_session_revoke_input", false},
		{"revoke failure", &fakeSessionRevoker{err: errors.New("boom")}, `{"kind":"connection","connection_id":"4"}`, "session_revoke_failed", true},
		{"no revoker", nil, `{"kind":"connection","connection_id":"4"}`, "session_revoke_failed", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := SessionRevokeHandler(tc.revoker).Handle(context.Background(), revokeRuntime(tc.input))
			var coded CodedError
			if !errors.As(err, &coded) || coded.Code != tc.code || coded.Retryable != tc.retryable {
				t.Fatalf("err = %#v, want code %s retryable %v", err, tc.code, tc.retryable)
			}
		})
	}
}

func TestSessionRevokeHandlerSkipsRevokerForUnknownKind(t *testing.T) {
	fake := &fakeSessionRevoker{}
	_, _ = SessionRevokeHandler(fake).Handle(context.Background(), revokeRuntime(`{"kind":"nope"}`))
	if len(fake.calls) != 0 {
		t.Fatalf("revoker called for unknown kind: %v", fake.calls)
	}
}
