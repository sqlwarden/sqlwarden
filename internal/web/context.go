package web

import (
	"context"
	"net/http"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/identity"
	"github.com/sqlwarden/internal/settings"
)

type contextKey string

const (
	authenticatedAccountKey contextKey = "authenticatedAccount"
	authSessionKey          contextKey = "authSession"
	pathIDsKey              contextKey = "pathIDs"
	requestLogContextKey    contextKey = "requestLogContext"
	runtimeSettingsKey      contextKey = "runtimeSettings"
	principalKey            contextKey = "principal"
)

func contextSetRuntimeSettings(r *http.Request, effective settings.Effective) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), runtimeSettingsKey, effective))
}

func contextGetRuntimeSettings(r *http.Request) (settings.Effective, bool) {
	effective, ok := r.Context().Value(runtimeSettingsKey).(settings.Effective)
	return effective, ok
}

// Account context helpers.
func contextSetAccount(r *http.Request, account identity.Account) *http.Request {
	if meta := contextGetRequestLogContext(r); meta != nil {
		meta.AccountID = account.ID
	}
	ctx := context.WithValue(r.Context(), authenticatedAccountKey, account)
	return r.WithContext(ctx)
}

func contextGetAccount(r *http.Request) identity.Account {
	account, _ := r.Context().Value(authenticatedAccountKey).(identity.Account)
	return account
}

// Principal context helpers.
func contextSetPrincipal(r *http.Request, p access.Principal) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), principalKey, p))
}

func contextGetPrincipal(r *http.Request) (access.Principal, bool) {
	p, ok := r.Context().Value(principalKey).(access.Principal)
	return p, ok
}

// Auth session context helpers.
func contextSetAuthSession(r *http.Request, session identity.AuthSession) *http.Request {
	if meta := contextGetRequestLogContext(r); meta != nil {
		meta.AuthSessionID = session.ID
	}
	ctx := context.WithValue(r.Context(), authSessionKey, session)
	return r.WithContext(ctx)
}

func contextGetAuthSession(r *http.Request) identity.AuthSession {
	session, _ := r.Context().Value(authSessionKey).(identity.AuthSession)
	return session
}

// pathIDs are the resource ids a route addresses. They are parsed from the
// path and carry no authorization: services resolve and authorize each id,
// including whether it belongs to its parent. A zero id is absent from the
// route.
type pathIDs struct {
	OrgID         int64
	OrgSlug       string
	WorkspaceID   int64
	EnvironmentID int64
	ConnectionID  int64
}

func contextSetPathIDs(r *http.Request, ids pathIDs) *http.Request {
	if meta := contextGetRequestLogContext(r); meta != nil {
		meta.OrgID = ids.OrgID
		meta.OrgSlug = ids.OrgSlug
		meta.WorkspaceID = ids.WorkspaceID
		meta.EnvironmentID = ids.EnvironmentID
		meta.ConnectionID = ids.ConnectionID
	}
	return r.WithContext(context.WithValue(r.Context(), pathIDsKey, ids))
}

func contextGetPathIDs(r *http.Request) pathIDs {
	ids, _ := r.Context().Value(pathIDsKey).(pathIDs)
	return ids
}

// Request log context helpers.
func contextSetRequestLogContext(r *http.Request, meta *requestLogContext) *http.Request {
	ctx := context.WithValue(r.Context(), requestLogContextKey, meta)
	return r.WithContext(ctx)
}

func contextGetRequestLogContext(r *http.Request) *requestLogContext {
	meta, _ := r.Context().Value(requestLogContextKey).(*requestLogContext)
	return meta
}
