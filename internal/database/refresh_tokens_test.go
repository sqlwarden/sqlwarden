package database

import (
	"context"
	"testing"
	"time"

	"github.com/sqlwarden/internal/assert"
)

func TestInsertAndGetRefreshToken(t *testing.T) {
	drivers := []string{"postgres", "sqlite"}

	for _, driver := range drivers {
		t.Run(driver+": Insert, fetch, and revoke", func(t *testing.T) {
			db := newTestDB(t, driver)

			pw := "hashed"
			account, err := db.InsertAccount(context.Background(), "rt@example.com", "RT User", &pw)
			assert.Nil(t, err)

			expires := time.Now().Add(24 * time.Hour)
			token, err := db.InsertRefreshToken(context.Background(), account.ID, "hash123", "family-abc", expires, "Mozilla/5.0", "192.168.1.1")
			assert.Nil(t, err)
			assert.Equal(t, token.AccountID, account.ID)
			assert.Equal(t, token.TokenHash, "hash123")
			assert.Equal(t, token.Family, "family-abc")
			assert.True(t, token.RevokedAt == nil)

			fetched, found, err := db.GetRefreshTokenByHash(context.Background(), "hash123")
			assert.Nil(t, err)
			assert.True(t, found)
			assert.Equal(t, fetched.ID, token.ID)
			assert.Equal(t, fetched.UserAgent, "Mozilla/5.0")
			assert.Equal(t, fetched.IPAddress, "192.168.1.1")

			// Revoke
			err = db.RevokeRefreshToken(context.Background(), token.ID)
			assert.Nil(t, err)

			revoked, found, err := db.GetRefreshTokenByHash(context.Background(), "hash123")
			assert.Nil(t, err)
			assert.True(t, found)
			assert.True(t, revoked.RevokedAt != nil)
		})

		t.Run(driver+": Non-existent hash returns not found", func(t *testing.T) {
			db := newTestDB(t, driver)

			_, found, err := db.GetRefreshTokenByHash(context.Background(), "nonexistent")
			assert.Nil(t, err)
			assert.False(t, found)
		})
	}
}

func TestRevokeFamilyTokens(t *testing.T) {
	drivers := []string{"postgres", "sqlite"}

	for _, driver := range drivers {
		t.Run(driver+": Revokes all tokens in family", func(t *testing.T) {
			db := newTestDB(t, driver)

			pw := "hashed"
			account, err := db.InsertAccount(context.Background(), "family@example.com", "Family User", &pw)
			assert.Nil(t, err)

			expires := time.Now().Add(24 * time.Hour)
			_, err = db.InsertRefreshToken(context.Background(), account.ID, "hash-a", "family-x", expires, "", "")
			assert.Nil(t, err)
			_, err = db.InsertRefreshToken(context.Background(), account.ID, "hash-b", "family-x", expires, "", "")
			assert.Nil(t, err)
			_, err = db.InsertRefreshToken(context.Background(), account.ID, "hash-c", "family-y", expires, "", "")
			assert.Nil(t, err)

			err = db.RevokeFamilyTokens(context.Background(), "family-x")
			assert.Nil(t, err)

			tokenA, _, err := db.GetRefreshTokenByHash(context.Background(), "hash-a")
			assert.Nil(t, err)
			assert.True(t, tokenA.RevokedAt != nil)

			tokenB, _, err := db.GetRefreshTokenByHash(context.Background(), "hash-b")
			assert.Nil(t, err)
			assert.True(t, tokenB.RevokedAt != nil)

			// family-y should be untouched
			tokenC, _, err := db.GetRefreshTokenByHash(context.Background(), "hash-c")
			assert.Nil(t, err)
			assert.True(t, tokenC.RevokedAt == nil)
		})
	}
}

func TestDeleteExpiredRefreshTokens(t *testing.T) {
	drivers := []string{"postgres", "sqlite"}

	for _, driver := range drivers {
		t.Run(driver+": Deletes expired tokens", func(t *testing.T) {
			db := newTestDB(t, driver)

			pw := "hashed"
			account, err := db.InsertAccount(context.Background(), "expired@example.com", "Expired User", &pw)
			assert.Nil(t, err)

			pastExpiry := time.Now().Add(-1 * time.Hour)
			futureExpiry := time.Now().Add(24 * time.Hour)

			_, err = db.InsertRefreshToken(context.Background(), account.ID, "expired-hash", "fam-e", pastExpiry, "", "")
			assert.Nil(t, err)
			_, err = db.InsertRefreshToken(context.Background(), account.ID, "valid-hash", "fam-v", futureExpiry, "", "")
			assert.Nil(t, err)

			err = db.DeleteExpiredRefreshTokens(context.Background())
			assert.Nil(t, err)

			_, found, err := db.GetRefreshTokenByHash(context.Background(), "expired-hash")
			assert.Nil(t, err)
			assert.False(t, found)

			_, found, err = db.GetRefreshTokenByHash(context.Background(), "valid-hash")
			assert.Nil(t, err)
			assert.True(t, found)
		})
	}
}
