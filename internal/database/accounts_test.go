package database

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/sqlwarden/internal/assert"
)

func TestInsertAndGetAccount(t *testing.T) {
	drivers := []string{"postgres", "sqlite"}

	for _, driver := range drivers {
		t.Run(driver+": Insert and fetch by ID", func(t *testing.T) {
			db := newTestDB(t, driver)

			pw := "hashed_password_123"
			account, err := db.InsertAccount(context.Background(), "test@example.com", "Test User", &pw)
			assert.Nil(t, err)
			assert.Equal(t, account.Email, "test@example.com")
			assert.Equal(t, account.Name, "Test User")
			assert.True(t, account.IsActive)

			fetched, found, err := db.GetAccount(context.Background(), account.ID)
			assert.Nil(t, err)
			assert.True(t, found)
			assert.Equal(t, fetched.ID, account.ID)
			assert.Equal(t, fetched.Email, "test@example.com")
			assert.Equal(t, fetched.Name, "Test User")
		})

		t.Run(driver+": Get non-existent account returns not found", func(t *testing.T) {
			db := newTestDB(t, driver)

			_, found, err := db.GetAccount(context.Background(), 999999)
			assert.Nil(t, err)
			assert.False(t, found)
		})

		t.Run(driver+": Insert with nil password", func(t *testing.T) {
			db := newTestDB(t, driver)

			account, err := db.InsertAccount(context.Background(), "sso@example.com", "SSO User", nil)
			assert.Nil(t, err)
			assert.True(t, account.Password == nil)

			fetched, found, err := db.GetAccount(context.Background(), account.ID)
			assert.Nil(t, err)
			assert.True(t, found)
			assert.True(t, fetched.Password == nil)
		})
	}
}

func TestGetAccountByEmail(t *testing.T) {
	drivers := []string{"postgres", "sqlite"}

	for _, driver := range drivers {
		t.Run(driver+": Case-insensitive email lookup", func(t *testing.T) {
			db := newTestDB(t, driver)

			pw := "hashed"
			_, err := db.InsertAccount(context.Background(), "Alice@Example.Com", "Alice", &pw)
			assert.Nil(t, err)

			account, found, err := db.GetAccountByEmail(context.Background(), "alice@example.com")
			assert.Nil(t, err)
			assert.True(t, found)
			assert.Equal(t, account.Name, "Alice")

			account2, found2, err := db.GetAccountByEmail(context.Background(), "ALICE@EXAMPLE.COM")
			assert.Nil(t, err)
			assert.True(t, found2)
			assert.Equal(t, account2.Name, "Alice")
		})

		t.Run(driver+": Non-existent email returns not found", func(t *testing.T) {
			db := newTestDB(t, driver)

			_, found, err := db.GetAccountByEmail(context.Background(), "nobody@example.com")
			assert.Nil(t, err)
			assert.False(t, found)
		})
	}
}

func TestDeactivateAccount(t *testing.T) {
	drivers := []string{"postgres", "sqlite"}

	for _, driver := range drivers {
		t.Run(driver+": Sets is_active to false", func(t *testing.T) {
			db := newTestDB(t, driver)

			pw := "hashed"
			account, err := db.InsertAccount(context.Background(), "deactivate@example.com", "Deactivate Me", &pw)
			assert.Nil(t, err)
			assert.True(t, account.IsActive)

			err = db.DeactivateAccount(context.Background(), account.ID)
			assert.Nil(t, err)

			fetched, found, err := db.GetAccount(context.Background(), account.ID)
			assert.Nil(t, err)
			assert.True(t, found)
			assert.False(t, fetched.IsActive)
		})
	}
}

func TestListAccountsPage(t *testing.T) {
	drivers := []string{"postgres", "sqlite"}

	for _, driver := range drivers {
		t.Run(driver+": Supports search sort and pagination", func(t *testing.T) {
			db := newTestDB(t, driver)

			pw := "hashed"
			_, err := db.InsertAccount(context.Background(), "zeta-user@example.com", "Zeta User", &pw)
			assert.Nil(t, err)
			_, err = db.InsertAccount(context.Background(), "alpha-user@example.com", "Alpha User", &pw)
			assert.Nil(t, err)
			_, err = db.InsertAccount(context.Background(), "beta-user@example.com", "Beta User", &pw)
			assert.Nil(t, err)

			result, err := db.ListAccountsPage(context.Background(), ListAccountsParams{
				Search:   "user",
				Sort:     "email",
				Order:    "asc",
				Page:     1,
				PageSize: 2,
			})
			assert.Nil(t, err)
			assert.Equal(t, result.Total, 3)
			assert.Equal(t, len(result.Items), 2)
			assert.Equal(t, result.Items[0].Email, "alpha-user@example.com")
			assert.Equal(t, result.Items[1].Email, "beta-user@example.com")
		})
	}
}

func TestAccountPasswordAbsentFromJSON(t *testing.T) {
	pw := "secret"
	account := Account{
		ID:       1,
		Email:    "test@example.com",
		Name:     "Test",
		Password: &pw,
		IsActive: true,
	}

	data, err := json.Marshal(account)
	assert.Nil(t, err)

	var m map[string]any
	err = json.Unmarshal(data, &m)
	assert.Nil(t, err)

	_, hasPassword := m["password"]
	assert.False(t, hasPassword)
}

func TestUpdateAccountPassword(t *testing.T) {
	drivers := []string{"postgres", "sqlite"}

	for _, driver := range drivers {
		t.Run(driver+": Updates password", func(t *testing.T) {
			db := newTestDB(t, driver)

			pw := "old_hash"
			account, err := db.InsertAccount(context.Background(), "pw-update@example.com", "PW User", &pw)
			assert.Nil(t, err)

			err = db.UpdateAccountPassword(context.Background(), account.ID, "new_hash")
			assert.Nil(t, err)

			fetched, found, err := db.GetAccount(context.Background(), account.ID)
			assert.Nil(t, err)
			assert.True(t, found)
			assert.True(t, fetched.Password != nil)
			assert.Equal(t, *fetched.Password, "new_hash")
		})
	}
}

func TestInsertAccountDuplicateEmail(t *testing.T) {
	drivers := []string{"postgres", "sqlite"}

	for _, driver := range drivers {
		t.Run(driver+": Duplicate email fails", func(t *testing.T) {
			db := newTestDB(t, driver)

			pw := "hashed"
			_, err := db.InsertAccount(context.Background(), "dup@example.com", "First", &pw)
			assert.Nil(t, err)

			_, err = db.InsertAccount(context.Background(), "dup@example.com", "Third", &pw)
			assert.NotNil(t, err)
		})
	}
}
