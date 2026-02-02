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
