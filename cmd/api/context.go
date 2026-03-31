package main

import (
	"context"
	"net/http"

	"github.com/sqlwarden/internal/database"
)

type contextKey string

const (
	authenticatedAccountKey contextKey = "authenticatedAccount"
	orgKey                  contextKey = "org"
	workspaceKey            contextKey = "workspace"
	environmentKey          contextKey = "environment"
	connectionKey           contextKey = "connection"
)

// Account context helpers.
func contextSetAccount(r *http.Request, account database.Account) *http.Request {
	ctx := context.WithValue(r.Context(), authenticatedAccountKey, account)
	return r.WithContext(ctx)
}

func contextGetAccount(r *http.Request) database.Account {
	account, _ := r.Context().Value(authenticatedAccountKey).(database.Account)
	return account
}

// Org context helpers.
func contextSetOrg(r *http.Request, org database.Organization) *http.Request {
	ctx := context.WithValue(r.Context(), orgKey, org)
	return r.WithContext(ctx)
}

func contextGetOrg(r *http.Request) database.Organization {
	org, _ := r.Context().Value(orgKey).(database.Organization)
	return org
}

// Workspace context helpers.
func contextSetWorkspace(r *http.Request, ws database.Workspace) *http.Request {
	ctx := context.WithValue(r.Context(), workspaceKey, ws)
	return r.WithContext(ctx)
}

func contextGetWorkspace(r *http.Request) database.Workspace {
	ws, _ := r.Context().Value(workspaceKey).(database.Workspace)
	return ws
}

// Environment context helpers.
func contextSetEnvironment(r *http.Request, env database.Environment) *http.Request {
	ctx := context.WithValue(r.Context(), environmentKey, env)
	return r.WithContext(ctx)
}

func contextGetEnvironment(r *http.Request) database.Environment {
	env, _ := r.Context().Value(environmentKey).(database.Environment)
	return env
}

// Connection context helpers.
func contextSetConnection(r *http.Request, conn database.Connection) *http.Request {
	ctx := context.WithValue(r.Context(), connectionKey, conn)
	return r.WithContext(ctx)
}

func contextGetConnection(r *http.Request) database.Connection {
	conn, _ := r.Context().Value(connectionKey).(database.Connection)
	return conn
}
