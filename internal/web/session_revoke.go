package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/sqlwarden/internal/jobs"
)

func (app *application) applySessionRevoke(ctx context.Context, in jobs.SessionRevokeInput) (int, error) {
	if app.revoker == nil {
		return 0, errors.New("session revoker is not configured")
	}
	switch in.Kind {
	case jobs.SessionRevokeConnection:
		return app.revoker.RevokeConnection(ctx, in.ConnectionID)
	case jobs.SessionRevokeWorkspaceAccount:
		return app.revoker.RevokeWorkspaceAccount(ctx, in.WorkspaceID, in.AccountID)
	case jobs.SessionRevokeOrgAccount:
		return app.revoker.RevokeOrgAccount(ctx, in.OrgID, in.AccountID)
	default:
		return 0, fmt.Errorf("unknown session revoke kind %q", in.Kind)
	}
}

func (app *application) handleSessionRevoke(ctx context.Context, runtime jobs.Runtime) (any, error) {
	var in jobs.SessionRevokeInput
	if err := json.Unmarshal([]byte(runtime.Job.InputJSON), &in); err != nil {
		return nil, jobs.Permanent("invalid_session_revoke_input", "Session revoke job input is invalid.")
	}
	switch in.Kind {
	case jobs.SessionRevokeConnection, jobs.SessionRevokeWorkspaceAccount, jobs.SessionRevokeOrgAccount:
	default:
		return nil, jobs.Permanent("invalid_session_revoke_input", "Session revoke job kind is unknown.")
	}
	revoked, err := app.applySessionRevoke(ctx, in)
	if err != nil {
		return nil, jobs.Retryable("session_revoke_failed", err.Error())
	}
	return map[string]any{"revoked": revoked}, nil
}
