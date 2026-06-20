package web

import (
	"context"
	"net/http"

	"github.com/sqlwarden/internal/database"
)

type contextKey string

const (
	authenticatedAccountKey contextKey = "authenticatedAccount"
	authSessionKey          contextKey = "authSession"
	orgKey                  contextKey = "org"
	workspaceKey            contextKey = "workspace"
	environmentKey          contextKey = "environment"
	connectionKey           contextKey = "connection"
	requestLogContextKey    contextKey = "requestLogContext"
)

// Account context helpers.
func contextSetAccount(r *http.Request, account database.Account) *http.Request {
	if meta := contextGetRequestLogContext(r); meta != nil {
		meta.AccountID = account.ID
	}
	ctx := context.WithValue(r.Context(), authenticatedAccountKey, account)
	return r.WithContext(ctx)
}

func contextGetAccount(r *http.Request) database.Account {
	account, _ := r.Context().Value(authenticatedAccountKey).(database.Account)
	return account
}

// Auth session context helpers.
func contextSetAuthSession(r *http.Request, session database.AuthSession) *http.Request {
	if meta := contextGetRequestLogContext(r); meta != nil {
		meta.AuthSessionID = session.ID
	}
	ctx := context.WithValue(r.Context(), authSessionKey, session)
	return r.WithContext(ctx)
}

func contextGetAuthSession(r *http.Request) database.AuthSession {
	session, _ := r.Context().Value(authSessionKey).(database.AuthSession)
	return session
}

// Org context helpers.
func contextSetOrg(r *http.Request, org database.Organization) *http.Request {
	if meta := contextGetRequestLogContext(r); meta != nil {
		meta.OrgID = org.ID
		meta.OrgSlug = org.Slug
	}
	ctx := context.WithValue(r.Context(), orgKey, org)
	return r.WithContext(ctx)
}

func contextGetOrg(r *http.Request) database.Organization {
	org, _ := r.Context().Value(orgKey).(database.Organization)
	return org
}

// Workspace context helpers.
func contextSetWorkspace(r *http.Request, ws database.Workspace) *http.Request {
	if meta := contextGetRequestLogContext(r); meta != nil {
		meta.WorkspaceID = ws.ID
	}
	ctx := context.WithValue(r.Context(), workspaceKey, ws)
	return r.WithContext(ctx)
}

func contextGetWorkspace(r *http.Request) database.Workspace {
	ws, _ := r.Context().Value(workspaceKey).(database.Workspace)
	return ws
}

// Environment context helpers.
func contextSetEnvironment(r *http.Request, env database.Environment) *http.Request {
	if meta := contextGetRequestLogContext(r); meta != nil {
		meta.EnvironmentID = env.ID
	}
	ctx := context.WithValue(r.Context(), environmentKey, env)
	return r.WithContext(ctx)
}

func contextGetEnvironment(r *http.Request) database.Environment {
	env, _ := r.Context().Value(environmentKey).(database.Environment)
	return env
}

// Connection context helpers.
func contextSetConnection(r *http.Request, conn database.Connection) *http.Request {
	if meta := contextGetRequestLogContext(r); meta != nil {
		meta.ConnectionID = conn.ID
	}
	ctx := context.WithValue(r.Context(), connectionKey, conn)
	return r.WithContext(ctx)
}

func contextGetConnection(r *http.Request) database.Connection {
	conn, _ := r.Context().Value(connectionKey).(database.Connection)
	return conn
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
