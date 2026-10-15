package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// SessionRevoker closes live target sessions. Implementations return the
// number of sessions they closed.
type SessionRevoker interface {
	RevokeConnection(ctx context.Context, connectionID string) (int, error)
	RevokeWorkspaceAccount(ctx context.Context, workspaceID, accountID string) (int, error)
	RevokeOrgAccount(ctx context.Context, orgID, accountID string) (int, error)
}

// SessionRevokeHandler runs session_revoke jobs against r. Undecodable input
// and unknown kinds fail permanently; a failed revocation is retryable.
func SessionRevokeHandler(r SessionRevoker) Handler {
	return HandlerFunc(func(ctx context.Context, runtime Runtime) (any, error) {
		var in SessionRevokeInput
		if err := json.Unmarshal([]byte(runtime.Job.InputJSON), &in); err != nil {
			return nil, Permanent("invalid_session_revoke_input", "Session revoke job input is invalid.")
		}
		switch in.Kind {
		case SessionRevokeConnection, SessionRevokeWorkspaceAccount, SessionRevokeOrgAccount:
		default:
			return nil, Permanent("invalid_session_revoke_input", "Session revoke job kind is unknown.")
		}
		revoked, err := applySessionRevoke(ctx, r, in)
		if err != nil {
			return nil, Retryable("session_revoke_failed", err.Error())
		}
		return map[string]any{"revoked": revoked}, nil
	})
}

func applySessionRevoke(ctx context.Context, r SessionRevoker, in SessionRevokeInput) (int, error) {
	if r == nil {
		return 0, errors.New("session revoker is not configured")
	}
	switch in.Kind {
	case SessionRevokeConnection:
		return r.RevokeConnection(ctx, in.ConnectionID)
	case SessionRevokeWorkspaceAccount:
		return r.RevokeWorkspaceAccount(ctx, in.WorkspaceID, in.AccountID)
	case SessionRevokeOrgAccount:
		return r.RevokeOrgAccount(ctx, in.OrgID, in.AccountID)
	default:
		return 0, fmt.Errorf("unknown session revoke kind %q", in.Kind)
	}
}
