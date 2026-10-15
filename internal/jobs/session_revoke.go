package jobs

import (
	"log/slog"
	"strings"
	"time"
)

// Session revoke kinds select which live target sessions a session_revoke job
// closes.
const (
	SessionRevokeConnection       = "connection"
	SessionRevokeWorkspaceAccount = "workspace_account"
	SessionRevokeOrgAccount       = "org_account"

	SessionRevokeMaxAttempts = 5
	// SessionRevokeSyncTimeout bounds the synchronous revocation attempt made
	// before a retry job is queued.
	SessionRevokeSyncTimeout = 30 * time.Second
)

// SessionRevokeInput is the session_revoke job payload. It carries
// identifiers only, so it is safe to persist.
type SessionRevokeInput struct {
	Kind         string `json:"kind"`
	ConnectionID string `json:"connection_id,omitempty"`
	WorkspaceID  string `json:"workspace_id,omitempty"`
	OrgID        string `json:"org_id,omitempty"`
	AccountID    string `json:"account_id,omitempty"`
}

// UniqueKey is the singleton key that collapses retries for the same target.
func (in SessionRevokeInput) UniqueKey() string {
	ids := []string{in.ConnectionID, in.WorkspaceID, in.OrgID, in.AccountID}
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != "" {
			parts = append(parts, id)
		}
	}
	return "session_revoke:" + in.Kind + ":" + strings.Join(parts, ":")
}

// LogAttrs returns the identifiers of the revocation as log attributes.
func (in SessionRevokeInput) LogAttrs() []slog.Attr {
	attrs := []slog.Attr{slog.String("revoke_kind", in.Kind)}
	for _, pair := range [][2]string{
		{"connection_id", in.ConnectionID}, {"workspace_id", in.WorkspaceID},
		{"org_id", in.OrgID}, {"target_account_id", in.AccountID},
	} {
		if pair[1] != "" {
			attrs = append(attrs, slog.String(pair[0], pair[1]))
		}
	}
	return attrs
}
