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
