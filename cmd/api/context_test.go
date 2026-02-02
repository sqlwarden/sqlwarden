package main

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/database"
)

func TestContextSetAuthenticatedUser(t *testing.T) {
	testUser := database.User{
		ID:             123,
		Created:        time.Now(),
		Email:          "alice@example.com",
		HashedPassword: "$2a$12$testhashedpassword",
	}

	t.Run("Returns new request with user set and original unchanged", func(t *testing.T) {
		originalReq := newTestRequest(t, http.MethodGet, "/test", nil)
		modifiedReq := contextSetAuthenticatedUser(originalReq, testUser)

		retrievedUser, found := originalReq.Context().Value(authenticatedUserContextKey).(database.User)
		assert.False(t, found)
		assert.Equal(t, retrievedUser, database.User{})

		retrievedUser, found = modifiedReq.Context().Value(authenticatedUserContextKey).(database.User)
		assert.True(t, found)
		assert.Equal(t, retrievedUser, testUser)
	})
}

func TestContextGetAuthenticatedUser(t *testing.T) {
	testUser := database.User{
		ID:             123,
		Created:        time.Now(),
		Email:          "alice@example.com",
		HashedPassword: "$2a$12$testhashedpassword",
	}

	t.Run("Successfully returns user when set", func(t *testing.T) {

		req := newTestRequest(t, http.MethodGet, "/test", nil)
		ctx := context.WithValue(req.Context(), authenticatedUserContextKey, testUser)
		req = req.WithContext(ctx)

		retrievedUser, found := contextGetAuthenticatedUser(req)
		assert.True(t, found)
		assert.Equal(t, retrievedUser, testUser)
	})

	t.Run("Returns zero user and false when not set", func(t *testing.T) {

		req := newTestRequest(t, http.MethodGet, "/test", nil)

		retrievedUser, found := contextGetAuthenticatedUser(req)
		assert.False(t, found)
		assert.Equal(t, retrievedUser, database.User{})
	})

	t.Run("Returns zero user and false if wrong type", func(t *testing.T) {

		req := newTestRequest(t, http.MethodGet, "/test", nil)
		ctx := context.WithValue(req.Context(), authenticatedUserContextKey, 123)
		req = req.WithContext(ctx)

		retrievedUser, found := contextGetAuthenticatedUser(req)
		assert.False(t, found)
		assert.Equal(t, retrievedUser, database.User{})
	})
}
