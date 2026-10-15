package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/sqlwarden/internal/jobs"
)

const (
	sessionRevokeConnection       = "connection"
	sessionRevokeWorkspaceAccount = "workspace_account"
	sessionRevokeOrgAccount       = "org_account"

	sessionRevokeMaxAttempts = 5
	sessionRevokeSyncTimeout = 30 * time.Second
)

// sessionRevokeInput carries identifiers only, so it is safe to persist as a
// job payload.
type sessionRevokeInput struct {
	Kind         string `json:"kind"`
	ConnectionID string `json:"connection_id,omitempty"`
	WorkspaceID  string `json:"workspace_id,omitempty"`
	OrgID        string `json:"org_id,omitempty"`
	AccountID    string `json:"account_id,omitempty"`
}

func (in sessionRevokeInput) uniqueKey() string {
	ids := []string{in.ConnectionID, in.WorkspaceID, in.OrgID, in.AccountID}
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != "" {
			parts = append(parts, id)
		}
	}
	return "session_revoke:" + in.Kind + ":" + strings.Join(parts, ":")
}

func (in sessionRevokeInput) logAttrs() []slog.Attr {
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

func (app *application) applySessionRevoke(ctx context.Context, in sessionRevokeInput) (int, error) {
	if app.revoker == nil {
		return 0, errors.New("session revoker is not configured")
	}
	switch in.Kind {
	case sessionRevokeConnection:
		return app.revoker.RevokeConnection(ctx, in.ConnectionID)
	case sessionRevokeWorkspaceAccount:
		return app.revoker.RevokeWorkspaceAccount(ctx, in.WorkspaceID, in.AccountID)
	case sessionRevokeOrgAccount:
		return app.revoker.RevokeOrgAccount(ctx, in.OrgID, in.AccountID)
	default:
		return 0, fmt.Errorf("unknown session revoke kind %q", in.Kind)
	}
}

// revokeSessions must run after the access change it enforces has committed.
// A failed revocation never changes the HTTP outcome: it is retried through a
// session_revoke job instead.
func (app *application) revokeSessions(r *http.Request, in sessionRevokeInput) {
	// The access change has already committed, so a client disconnect must
	// neither skip the revocation nor lose the retry.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), sessionRevokeSyncTimeout)
	defer cancel()
	_, err := app.applySessionRevoke(ctx, in)
	if err == nil {
		return
	}
	app.logWarn(r, "session revocation failed; queueing retry", append(in.logAttrs(), slog.String("error", err.Error()))...)
	_, _, err = app.workspaceJobStore().EnqueueSingleton(context.WithoutCancel(r.Context()), jobs.EnqueueInput{
		Type:         jobs.TypeSessionRevoke,
		SingletonKey: in.uniqueKey(),
		Visibility:   jobs.VisibilityInternal,
		Priority:     jobs.PriorityHigh,
		MaxAttempts:  sessionRevokeMaxAttempts,
		Input:        in,
	})
	switch {
	case err == nil:
	case errors.Is(err, jobs.ErrActiveExists):
		app.logDebug(r, "session revocation retry already queued", in.logAttrs()...)
	default:
		app.logWarn(r, "session revocation retry could not be queued", append(in.logAttrs(), slog.String("error", err.Error()))...)
	}
}

func (app *application) handleSessionRevoke(ctx context.Context, runtime jobs.Runtime) (any, error) {
	var in sessionRevokeInput
	if err := json.Unmarshal([]byte(runtime.Job.InputJSON), &in); err != nil {
		return nil, jobs.Permanent("invalid_session_revoke_input", "Session revoke job input is invalid.")
	}
	switch in.Kind {
	case sessionRevokeConnection, sessionRevokeWorkspaceAccount, sessionRevokeOrgAccount:
	default:
		return nil, jobs.Permanent("invalid_session_revoke_input", "Session revoke job kind is unknown.")
	}
	revoked, err := app.applySessionRevoke(ctx, in)
	if err != nil {
		return nil, jobs.Retryable("session_revoke_failed", err.Error())
	}
	return map[string]any{"revoked": revoked}, nil
}
