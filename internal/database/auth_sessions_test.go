package database

import (
	"context"
	"testing"
	"time"

	"github.com/sqlwarden/internal/assert"
)

func TestAuthSessionRecordsMethodAndAssurance(t *testing.T) {
	for _, driver := range []string{"postgres", "sqlite"} {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()
			pw := "hashed"
			account, err := db.InsertAccount(ctx, "method@example.com", "Method", &pw)
			assert.Nil(t, err)
			expires := time.Now().Add(time.Hour)

			inserted, err := db.InsertAuthSession(ctx, account.ID, expires, "agent", "127.0.0.1", SessionAuth{Method: "local", Assurance: "aal1"})
			assert.Nil(t, err)
			got, found, err := db.GetAuthSession(ctx, inserted.ID, account.ID)
			assert.Nil(t, err)
			assert.True(t, found)
			assert.Equal(t, got.AuthMethod, "local")
			assert.Equal(t, got.Assurance, "aal1")

			created, _, err := db.CreateAuthSessionWithRefreshToken(ctx, account.ID, expires, "agent", "127.0.0.1", "method-token-hash", "method-family", SessionAuth{Method: "password", Assurance: "aal1"})
			assert.Nil(t, err)
			got, found, err = db.GetAuthSession(ctx, created.ID, account.ID)
			assert.Nil(t, err)
			assert.True(t, found)
			assert.Equal(t, got.AuthMethod, "password")
			assert.Equal(t, got.Assurance, "aal1")
		})
	}
}

func TestInsertAuthSessionRequiresMethod(t *testing.T) {
	db := newTestDB(t, "sqlite")
	pw := "hashed"
	account, err := db.InsertAccount(context.Background(), "nomethod@example.com", "None", &pw)
	assert.Nil(t, err)
	_, err = db.InsertAuthSession(context.Background(), account.ID, time.Now().Add(time.Hour), "agent", "127.0.0.1", SessionAuth{})
	assert.NotNil(t, err)
}

func TestChangeAccountPasswordRevokesOtherSessions(t *testing.T) {
	db := newTestDB(t, "sqlite")
	ctx := context.Background()
	pw := "old-hash"
	account, err := db.InsertAccount(ctx, "rotate@example.com", "Rotate", &pw)
	assert.Nil(t, err)
	other, err := db.InsertAccount(ctx, "bystander@example.com", "Bystander", &pw)
	assert.Nil(t, err)
	expires := time.Now().Add(time.Hour)
	auth := SessionAuth{Method: "password", Assurance: "aal1"}

	current, _, err := db.CreateAuthSessionWithRefreshToken(ctx, account.ID, expires, "a", "127.0.0.1", "current-hash", "current-family", auth)
	assert.Nil(t, err)
	stale, _, err := db.CreateAuthSessionWithRefreshToken(ctx, account.ID, expires, "b", "127.0.0.1", "stale-hash", "stale-family", auth)
	assert.Nil(t, err)
	bystander, _, err := db.CreateAuthSessionWithRefreshToken(ctx, other.ID, expires, "c", "127.0.0.1", "bystander-hash", "bystander-family", auth)
	assert.Nil(t, err)

	assert.Nil(t, db.ChangeAccountPassword(ctx, account.ID, "new-hash", current.ID, "password_changed"))

	got, _, err := db.GetAccount(ctx, account.ID)
	assert.Nil(t, err)
	assert.Equal(t, *got.Password, "new-hash")

	kept, _, err := db.GetAuthSession(ctx, current.ID, account.ID)
	assert.Nil(t, err)
	assert.True(t, kept.RevokedAt == nil)
	revoked, _, err := db.GetAuthSession(ctx, stale.ID, account.ID)
	assert.Nil(t, err)
	assert.True(t, revoked.RevokedAt != nil)
	assert.Equal(t, revoked.RevocationReason, "password_changed")
	untouched, _, err := db.GetAuthSession(ctx, bystander.ID, other.ID)
	assert.Nil(t, err)
	assert.True(t, untouched.RevokedAt == nil)

	keptToken, _, err := db.GetRefreshTokenByHash(ctx, "current-hash")
	assert.Nil(t, err)
	assert.True(t, keptToken.RevokedAt == nil)
	staleToken, _, err := db.GetRefreshTokenByHash(ctx, "stale-hash")
	assert.Nil(t, err)
	assert.True(t, staleToken.RevokedAt != nil)
	bystanderToken, _, err := db.GetRefreshTokenByHash(ctx, "bystander-hash")
	assert.Nil(t, err)
	assert.True(t, bystanderToken.RevokedAt == nil)
}
