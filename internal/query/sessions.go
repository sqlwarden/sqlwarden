package query

import (
	"context"
	"strconv"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/execution"
)

// runtimePermissions are the connection permissions that each grant the use
// of a live session. Which statements a session may run is decided per
// statement class on top of this gate.
var runtimePermissions = []string{
	access.PermConnExecute,
	access.PermConnDQL,
	access.PermConnDML,
	access.PermConnDDL,
}

// scope is the execution scope of the principal on the routed connection. The
// runtime re-checks it against every session it is asked to use, so a session
// owned by another account or connection is indistinguishable from an
// unknown one.
func scope(p access.Principal, ref access.ConnRef) execution.Scope {
	return execution.Scope{
		OrgID:        strconv.FormatInt(ref.OrgID, 10),
		WorkspaceID:  strconv.FormatInt(ref.WorkspaceID, 10),
		AccountID:    strconv.FormatInt(p.Subject.ID, 10),
		ConnectionID: strconv.FormatInt(ref.ConnectionID, 10),
	}
}

func limits(policy Policy) execution.Limits {
	return execution.Limits{MaxRows: policy.MaxRows, MaxBytes: policy.MaxBytes}
}

// runtimeConnection authorizes use of a live session on the connection.
func (s *Service) runtimeConnection(ctx context.Context, p access.Principal, ref access.ConnRef) (access.Connection, access.Workspace, error) {
	return s.deps.Requirer.Connection(ctx, p, ref, runtimePermissions...)
}

// Connect opens, or reuses, the principal's session on the connection.
// Open failures are returned unchanged for the caller to classify.
func (s *Service) Connect(ctx context.Context, p access.Principal, ref access.ConnRef) (execution.SessionInfo, error) {
	_, ws, err := s.runtimeConnection(ctx, p, ref)
	if err != nil {
		return execution.SessionInfo{}, err
	}
	policy, err := s.deps.Settings.ForWorkspace(ctx, ws)
	if err != nil {
		return execution.SessionInfo{}, err
	}
	return s.deps.Runtime.Open(ctx, execution.OpenRequest{Scope: scope(p, ref), Limits: limits(policy)})
}

// Disconnect closes the principal's session. It is idempotent: a session that
// is already gone or belongs to another scope succeeds without being touched.
// It requires no connection permission so a principal whose access was
// narrowed can still release a session they hold.
func (s *Service) Disconnect(ctx context.Context, p access.Principal, ref access.ConnRef, id execution.SessionID) error {
	if _, _, err := s.deps.Requirer.ResolveConnection(ctx, p, ref); err != nil {
		return err
	}
	if id == "" {
		return ErrSessionRequired
	}
	return s.deps.Runtime.Close(ctx, scope(p, ref), id)
}

// ListWorkspaceSessions lists live sessions in a workspace the principal
// belongs to. Only the principal's own sessions are listed unless they hold
// policy:read on the workspace.
func (s *Service) ListWorkspaceSessions(ctx context.Context, p access.Principal, orgID, wsID int64) ([]execution.SessionInfo, error) {
	ws, err := s.deps.Requirer.WorkspaceMember(ctx, p, orgID, wsID)
	if err != nil {
		return nil, err
	}
	workspaceID := strconv.FormatInt(ws.ID, 10)
	filter := execution.SessionFilter{AccountID: strconv.FormatInt(p.Subject.ID, 10), WorkspaceID: workspaceID}
	if s.deps.Requirer.Can(ctx, p, orgID, ws.OwnerType, "workspace", ws.ID, access.PermPolicyRead) {
		filter.AccountID = ""
	}
	sessions, err := s.deps.Runtime.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	out := make([]execution.SessionInfo, 0, len(sessions))
	for _, session := range sessions {
		if session.Scope.WorkspaceID == workspaceID {
			out = append(out, session)
		}
	}
	return out, nil
}

// RevokeSession closes a session in the workspace. The session owner may
// always revoke it; anyone else needs policy:modify on the workspace. A
// session outside the workspace is indistinguishable from one that has
// already ended, so it reports nil with no error. The revoked session is
// returned so the caller can record who owned it.
func (s *Service) RevokeSession(ctx context.Context, p access.Principal, orgID, wsID int64, sessionID string) (*execution.SessionInfo, error) {
	ws, err := s.deps.Requirer.Workspace(ctx, p, orgID, wsID, "")
	if err != nil {
		return nil, err
	}
	sessions, err := s.deps.Runtime.List(ctx, execution.SessionFilter{WorkspaceID: strconv.FormatInt(ws.ID, 10)})
	if err != nil {
		return nil, err
	}
	var session *execution.SessionInfo
	for i := range sessions {
		if string(sessions[i].ID) == sessionID {
			session = &sessions[i]
			break
		}
	}
	if session == nil {
		return nil, nil
	}
	if session.Scope.AccountID != strconv.FormatInt(p.Subject.ID, 10) &&
		!s.deps.Requirer.Can(ctx, p, orgID, ws.OwnerType, "workspace", ws.ID, access.PermPolicyModify) {
		return nil, ErrForbidden
	}
	if err := s.deps.Runtime.Close(ctx, session.Scope, session.ID); err != nil {
		return nil, err
	}
	return session, nil
}
