package main

import (
	"context"
	"net/http"

	"github.com/sqlwarden/internal/database"
)

type contextKey string

const (
	authenticatedUserContextKey = contextKey("authenticatedUser")
)

func contextSetAuthenticatedUser(r *http.Request, user database.User) *http.Request {
	ctx := context.WithValue(r.Context(), authenticatedUserContextKey, user)
	return r.WithContext(ctx)
}

func contextGetAuthenticatedUser(r *http.Request) (database.User, bool) {
	user, ok := r.Context().Value(authenticatedUserContextKey).(database.User)
	return user, ok
}

const (
	accountContextKey   = contextKey("account")
	tenantContextKey    = contextKey("tenant")
	workspaceContextKey = contextKey("workspace")
)

func contextSetAccount(r *http.Request, account database.Account) *http.Request {
	ctx := context.WithValue(r.Context(), accountContextKey, account)
	return r.WithContext(ctx)
}

func contextGetAccount(r *http.Request) (database.Account, bool) {
	account, ok := r.Context().Value(accountContextKey).(database.Account)
	return account, ok
}

func contextSetTenant(r *http.Request, tenant database.Tenant) *http.Request {
	ctx := context.WithValue(r.Context(), tenantContextKey, tenant)
	return r.WithContext(ctx)
}

func contextGetTenant(r *http.Request) (database.Tenant, bool) {
	tenant, ok := r.Context().Value(tenantContextKey).(database.Tenant)
	return tenant, ok
}

func contextSetWorkspace(r *http.Request, ws database.Workspace) *http.Request {
	ctx := context.WithValue(r.Context(), workspaceContextKey, ws)
	return r.WithContext(ctx)
}

func contextGetWorkspace(r *http.Request) (database.Workspace, bool) {
	ws, ok := r.Context().Value(workspaceContextKey).(database.Workspace)
	return ws, ok
}
