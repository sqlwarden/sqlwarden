package main

import (
	"context"
	"net/http"
	"testing"

	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/database"
)

func TestContextSetAndGetAccount(t *testing.T) {
	testAccount := database.Account{
		ID:    123,
		Email: "alice@example.com",
		Name:  "Alice",
	}

	t.Run("Returns new request with account set and original unchanged", func(t *testing.T) {
		originalReq := newTestRequest(t, http.MethodGet, "/test", nil)
		modifiedReq := contextSetAccount(originalReq, testAccount)

		retrieved := contextGetAccount(originalReq)
		assert.Equal(t, retrieved.ID, int64(0))

		retrieved = contextGetAccount(modifiedReq)
		assert.Equal(t, retrieved.ID, testAccount.ID)
		assert.Equal(t, retrieved.Email, testAccount.Email)
	})
}

func TestContextGetAccountEmpty(t *testing.T) {
	t.Run("Returns zero account when not set", func(t *testing.T) {
		req := newTestRequest(t, http.MethodGet, "/test", nil)
		acc := contextGetAccount(req)
		assert.Equal(t, acc.ID, int64(0))
	})

	t.Run("Returns zero account if wrong type", func(t *testing.T) {
		req := newTestRequest(t, http.MethodGet, "/test", nil)
		ctx := context.WithValue(req.Context(), authenticatedAccountKey, 123)
		req = req.WithContext(ctx)
		acc := contextGetAccount(req)
		assert.Equal(t, acc.ID, int64(0))
	})
}
