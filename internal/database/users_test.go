package database

import (
	"strings"
	"testing"

	"github.com/sqlwarden/internal/assert"
)

func TestInsertUser(t *testing.T) {
	t.Run("Successfully inserts user and returns ID", func(t *testing.T) {
		db := newTestDB(t)

		testEmail := "test@example.com"
		testHashedPassword := "$2a$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/LewrBQ7Q/C0YQxK.6"

		id, err := db.InsertUser(testEmail, testHashedPassword)
		assert.Nil(t, err)
		assert.True(t, id > 0)

		var user User
		err = db.Get(&user, "SELECT * FROM users WHERE id = $1", id)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, user.Email, testEmail)
		assert.Equal(t, user.HashedPassword, testHashedPassword)
	})

	t.Run("Fails with duplicate email", func(t *testing.T) {
		db := newTestDB(t)

		id, err := db.InsertUser(testUsers["alice"].email, testUsers["alice"].hashedPassword)
		assert.NotNil(t, err)
		assert.Equal(t, id, 0)
	})
}

func TestGetUser(t *testing.T) {
	t.Run("Returns user when ID exists", func(t *testing.T) {
		db := newTestDB(t)

		user, found, err := db.GetUser(testUsers["alice"].id)
		assert.Nil(t, err)
		assert.True(t, found)
		assert.Equal(t, user.ID, testUsers["alice"].id)
		assert.Equal(t, user.Email, testUsers["alice"].email)
		assert.Equal(t, user.HashedPassword, testUsers["alice"].hashedPassword)
	})

	t.Run("Returns not found when ID does not exist", func(t *testing.T) {
		db := newTestDB(t)

		userID := 99999

		user, found, err := db.GetUser(userID)
		assert.Nil(t, err)
		assert.False(t, found)
		assert.Equal(t, user, User{})
	})
}

func TestGetUserByEmail(t *testing.T) {
	t.Run("Returns user when email exists", func(t *testing.T) {
		db := newTestDB(t)

		user, found, err := db.GetUserByEmail(testUsers["alice"].email)
		assert.Nil(t, err)
		assert.True(t, found)
		assert.Equal(t, user.ID, testUsers["alice"].id)
		assert.Equal(t, user.Email, testUsers["alice"].email)
		assert.Equal(t, user.HashedPassword, testUsers["alice"].hashedPassword)
	})

	t.Run("Returns not found when email does not exist", func(t *testing.T) {
		db := newTestDB(t)

		testEmail := "nonexistent@example.com"

		user, found, err := db.GetUserByEmail(testEmail)
		assert.Nil(t, err)
		assert.False(t, found)
		assert.Equal(t, user, User{})
	})

	t.Run("Is case-insensitive for lookup", func(t *testing.T) {
		db := newTestDB(t)

		_, found, err := db.GetUserByEmail(strings.ToUpper(testUsers["alice"].email))
		assert.Nil(t, err)
		assert.True(t, found)
	})
}

func TestUpdateUserHashedPassword(t *testing.T) {
	t.Run("Successfully updates user's hashed password", func(t *testing.T) {
		db := newTestDB(t)

		originalHashedPassword := testUsers["alice"].hashedPassword
		newHashedPassword := "$2a$12$EixZaYVK1fsbw1ZfbX3OXePaWxn96p36WQoeG6Lruj3vjPGga31lW"

		err := db.UpdateUserHashedPassword(testUsers["alice"].id, newHashedPassword)
		assert.Nil(t, err)

		user, found, err := db.GetUser(testUsers["alice"].id)
		if err != nil {
			t.Fatal(err)
		}
		assert.True(t, found)
		assert.Equal(t, newHashedPassword, user.HashedPassword)
		assert.True(t, user.HashedPassword != originalHashedPassword)
		assert.Equal(t, user.Email, testUsers["alice"].email)

		user2, _, err := db.GetUser(testUsers["bob"].id)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, user2.HashedPassword, testUsers["bob"].hashedPassword)
		assert.NotEqual(t, user2.HashedPassword, user.HashedPassword)
	})

	t.Run("Does not error when user ID does not exist", func(t *testing.T) {
		db := newTestDB(t)

		userID := 99999
		newPassword := "$2a$12$EixZaYVK1fsbw1ZfbX3OXePaWxn96p36WQoeG6Lruj3vjPGga31lW"

		err := db.UpdateUserHashedPassword(userID, newPassword)
		assert.Nil(t, err)
	})
}
