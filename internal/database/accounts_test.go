package database

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/oklog/ulid/v2"
	"github.com/sqlwarden/internal/assert"
)

func TestInsertAndGetAccount(t *testing.T) {
	drivers := []string{"postgres", "sqlite"}

	for _, driver := range drivers {
		t.Run(driver+": Insert and fetch by ID", func(t *testing.T) {
			db := newTestDB(t, driver)

			pw := "hashed_password_123"
			account, err := db.InsertAccount("test@example.com", "Test User", &pw)
			assert.Nil(t, err)
			assert.Equal(t, account.Email, "test@example.com")
			assert.Equal(t, account.Name, "Test User")
			assert.True(t, account.IsActive)

			fetched, found, err := db.GetAccount(account.ID)
			assert.Nil(t, err)
			assert.True(t, found)
			assert.Equal(t, fetched.ID, account.ID)
			assert.Equal(t, fetched.Email, "test@example.com")
			assert.Equal(t, fetched.Name, "Test User")
		})

		t.Run(driver+": Get non-existent account returns not found", func(t *testing.T) {
			db := newTestDB(t, driver)

			_, found, err := db.GetAccount("nonexistent")
			assert.Nil(t, err)
			assert.False(t, found)
		})

		t.Run(driver+": Insert with nil password", func(t *testing.T) {
			db := newTestDB(t, driver)

			account, err := db.InsertAccount("sso@example.com", "SSO User", nil)
			assert.Nil(t, err)
			assert.True(t, account.Password == nil)

			fetched, found, err := db.GetAccount(account.ID)
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
			_, err := db.InsertAccount("Alice@Example.Com", "Alice", &pw)
			assert.Nil(t, err)

			account, found, err := db.GetAccountByEmail("alice@example.com")
			assert.Nil(t, err)
			assert.True(t, found)
			assert.Equal(t, account.Name, "Alice")

			account2, found2, err := db.GetAccountByEmail("ALICE@EXAMPLE.COM")
			assert.Nil(t, err)
			assert.True(t, found2)
			assert.Equal(t, account2.Name, "Alice")
		})

		t.Run(driver+": Non-existent email returns not found", func(t *testing.T) {
			db := newTestDB(t, driver)

			_, found, err := db.GetAccountByEmail("nobody@example.com")
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
			account, err := db.InsertAccount("deactivate@example.com", "Deactivate Me", &pw)
			assert.Nil(t, err)
			assert.True(t, account.IsActive)

			err = db.DeactivateAccount(account.ID)
			assert.Nil(t, err)

			fetched, found, err := db.GetAccount(account.ID)
			assert.Nil(t, err)
			assert.True(t, found)
			assert.False(t, fetched.IsActive)
		})
	}
}

func TestAccountPasswordAbsentFromJSON(t *testing.T) {
	pw := "secret"
	account := Account{
		ID:       "test",
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

func TestAccountIDIsValidULID(t *testing.T) {
	drivers := []string{"postgres", "sqlite"}

	for _, driver := range drivers {
		t.Run(driver+": Returned ID is a valid ULID", func(t *testing.T) {
			db := newTestDB(t, driver)

			pw := "hashed"
			account, err := db.InsertAccount("ulid-test@example.com", "ULID Test", &pw)
			assert.Nil(t, err)
			assert.Equal(t, len(account.ID), 26)

			_, err = ulid.Parse(account.ID)
			assert.Nil(t, err)
		})
	}
}

func TestUpdateAccountPassword(t *testing.T) {
	drivers := []string{"postgres", "sqlite"}

	for _, driver := range drivers {
		t.Run(driver+": Updates password", func(t *testing.T) {
			db := newTestDB(t, driver)

			pw := "old_hash"
			account, err := db.InsertAccount("pw-update@example.com", "PW User", &pw)
			assert.Nil(t, err)

			err = db.UpdateAccountPassword(account.ID, "new_hash")
			assert.Nil(t, err)

			fetched, found, err := db.GetAccount(account.ID)
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
			_, err := db.InsertAccount("dup@example.com", "First", &pw)
			assert.Nil(t, err)

			_, _ = db.InsertAccount(strings.ToUpper("dup@example.com"), "Second", &pw)
			// Note: unique constraint is on the raw email column, not LOWER(email),
			// so same-case duplicates fail. Depending on DB, different case may or may not fail.
			// We just test same-case here.
			_, err = db.InsertAccount("dup@example.com", "Third", &pw)
			assert.NotNil(t, err)
		})
	}
}

func TestIsSuperadminDefaultsFalse(t *testing.T) {
	drivers := []string{"postgres", "sqlite"}

	for _, driver := range drivers {
		t.Run(driver+": IsSuperadmin defaults to false", func(t *testing.T) {
			db := newTestDB(t, driver)

			pw := "hashed"
			account, err := db.InsertAccount("superadmin-test@example.com", "Super Test", &pw)
			assert.Nil(t, err)
			assert.False(t, account.IsSuperadmin)

			fetched, found, err := db.GetAccount(account.ID)
			assert.Nil(t, err)
			assert.True(t, found)
			assert.False(t, fetched.IsSuperadmin)
		})
	}
}

func TestListAllAccounts(t *testing.T) {
	drivers := []string{"postgres", "sqlite"}

	for _, driver := range drivers {
		t.Run(driver+": Returns paginated accounts with total count", func(t *testing.T) {
			db := newTestDB(t, driver)

			pw := "hashed"
			_, err := db.InsertAccount("list1@example.com", "List One", &pw)
			assert.Nil(t, err)
			_, err = db.InsertAccount("list2@example.com", "List Two", &pw)
			assert.Nil(t, err)
			_, err = db.InsertAccount("list3@example.com", "List Three", &pw)
			assert.Nil(t, err)

			accounts, total, err := db.ListAllAccounts(1, 10)
			assert.Nil(t, err)
			assert.Equal(t, total, 3)
			assert.Equal(t, len(accounts), 3)
		})

		t.Run(driver+": Pagination limits results", func(t *testing.T) {
			db := newTestDB(t, driver)

			pw := "hashed"
			_, err := db.InsertAccount("page1@example.com", "Page One", &pw)
			assert.Nil(t, err)
			_, err = db.InsertAccount("page2@example.com", "Page Two", &pw)
			assert.Nil(t, err)
			_, err = db.InsertAccount("page3@example.com", "Page Three", &pw)
			assert.Nil(t, err)

			accounts, total, err := db.ListAllAccounts(1, 2)
			assert.Nil(t, err)
			assert.Equal(t, total, 3)
			assert.Equal(t, len(accounts), 2)

			// page 2 has the remaining 1
			accounts2, total2, err := db.ListAllAccounts(2, 2)
			assert.Nil(t, err)
			assert.Equal(t, total2, 3)
			assert.Equal(t, len(accounts2), 1)
		})

		t.Run(driver+": Empty accounts table returns zero count", func(t *testing.T) {
			db := newTestDB(t, driver)

			// newTestDB does not insert into the accounts table
			accounts, total, err := db.ListAllAccounts(1, 10)
			assert.Nil(t, err)
			assert.Equal(t, total, 0)
			assert.Equal(t, len(accounts), 0)
		})
	}
}
