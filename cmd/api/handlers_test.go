package main

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/sqlwarden/internal/assert"
)

func TestStatus(t *testing.T) {
	t.Run("GET renders the status response", func(t *testing.T) {
		app := newTestApplication(t)

		req := newTestRequest(t, http.MethodGet, "/status", nil)

		res := send(t, req, app.routes())
		assert.Equal(t, res.StatusCode, http.StatusOK)
		assert.Equal(t, res.BodyFields["Status"], "OK")
	})
}

func TestCreateUser(t *testing.T) {
	t.Run("Creates a new user", func(t *testing.T) {
		app := newTestApplication(t)

		req := newTestRequest(t, http.MethodPost, "/users", map[string]any{
			"Email":    "zara@example.com",
			"Password": "Zara_pw_fake00",
		})

		res := send(t, req, app.routes())
		assert.Equal(t, res.StatusCode, http.StatusNoContent)

		user, found, err := app.db.GetUserByEmail("zara@example.com")
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, found, true)
		assert.MatchesRegexp(t, user.HashedPassword, `^\$2a\$12\$[./0-9A-Za-z]{53}$`)
	})

	t.Run("POST rejects invalid data and re-displays the form", func(t *testing.T) {
		tests := []struct {
			testName     string
			userEmail    string
			userPassword string
		}{
			{
				testName:     "Rejects empty email",
				userEmail:    "",
				userPassword: "demo789$Test",
			},
			{
				testName:     "Rejects empty password",
				userEmail:    "zoe@example.com",
				userPassword: "",
			},
			{
				testName:     "Rejects invalid email",
				userEmail:    "invalid@example.",
				userPassword: "demo789$Test",
			},
			{
				testName:     "Rejects short password",
				userEmail:    "zoe@example.com",
				userPassword: "k4k3dw9",
			},
			{
				testName:     "Rejects password longer than 72 bytes",
				userEmail:    "zoe@example.com",
				userPassword: "iRbMr5Av5T1DINST1l2pGBBUtW4Qn628N4lN6tFNjW8Ea4fuYiI84j2KH8tKQrF3INkqbKwZh",
			},
			{
				testName:     "Rejects common password",
				userEmail:    "zoe@example.com",
				userPassword: "sunshine",
			},
			{
				testName:     "Rejects duplicate user",
				userEmail:    testUsers["alice"].email,
				userPassword: "pw-fake-abc987",
			},
		}

		for _, tt := range tests {
			t.Run(tt.testName, func(t *testing.T) {
				app := newTestApplication(t)

				req := newTestRequest(t, http.MethodPost, "/users", map[string]any{
					"Email":    tt.userEmail,
					"Password": tt.userPassword,
				})

				res := send(t, req, app.routes())
				assert.Equal(t, res.StatusCode, http.StatusUnprocessableEntity)
			})
		}
	})
}

func TestCreateAuthenticationToken(t *testing.T) {
	t.Run("Creates an authentication token for a valid user", func(t *testing.T) {
		app := newTestApplication(t)

		req := newTestRequest(t, http.MethodPost, "/authentication-tokens", map[string]any{
			"Email":    testUsers["alice"].email,
			"Password": testUsers["alice"].password,
		})

		res := send(t, req, app.routes())
		assert.Equal(t, res.StatusCode, http.StatusOK)
		assert.MatchesRegexp(t, res.BodyFields["AuthenticationToken"].(string), `^eyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$`)
		assert.MatchesRegexp(t, res.BodyFields["AuthenticationTokenExpiry"].(string), `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`)
	})

	t.Run("Rejects invalid credentials", func(t *testing.T) {
		tests := []struct {
			testName     string
			userEmail    string
			userPassword string
		}{
			{
				testName:     "Rejects empty email",
				userEmail:    "",
				userPassword: testUsers["alice"].password,
			},
			{
				testName:     "Rejects empty password",
				userEmail:    testUsers["alice"].email,
				userPassword: "",
			},
			{
				testName:     "Rejects valid email but invalid password",
				userEmail:    testUsers["alice"].email,
				userPassword: "NotARealPass123#",
			},
			{
				testName:     "Rejects invalid email but valid password",
				userEmail:    "zaha@example.com",
				userPassword: testUsers["alice"].password,
			},
			{
				testName:     "Rejects mismatched email and password",
				userEmail:    testUsers["alice"].email,
				userPassword: testUsers["bob"].password,
			},
		}

		for _, tt := range tests {
			t.Run(tt.testName, func(t *testing.T) {
				app := newTestApplication(t)

				req := newTestRequest(t, http.MethodPost, "/authentication-tokens", map[string]any{
					"Email":    tt.userEmail,
					"Password": tt.userPassword,
				})

				res := send(t, req, app.routes())
				assert.Equal(t, res.StatusCode, http.StatusUnprocessableEntity)
			})
		}
	})
}

func TestRestricted(t *testing.T) {
	t.Run("Unauthenticated users get a 401 response and error message", func(t *testing.T) {
		app := newTestApplication(t)

		req := newTestRequest(t, http.MethodGet, "/restricted", nil)

		res := send(t, req, app.routes())
		assert.Equal(t, res.StatusCode, http.StatusUnauthorized)
		assert.Equal(t, res.BodyFields["Error"], "You must be authenticated to access this resource")
	})

	t.Run("Authenticated users get a 200 response", func(t *testing.T) {
		app := newTestApplication(t)

		jwt, _, err := app.newAuthenticationToken(testUsers["alice"].id)
		if err != nil {
			t.Fatal(err)
		}

		req := newTestRequest(t, http.MethodGet, "/restricted", nil)
		req.Header.Set("Authorization", "Bearer "+jwt)

		res := send(t, req, app.routes())
		assert.Equal(t, res.StatusCode, http.StatusOK)
		assert.Equal(t, res.BodyFields["Message"], "This is a restricted handler")
	})
}

func TestGetUsers(t *testing.T) {
	t.Run("returns all users", func(t *testing.T) {
		app := newTestApplication(t)

		req := newTestRequest(t, http.MethodGet, "/users", nil)
		res := send(t, req, app.routes())

		assert.Equal(t, res.StatusCode, http.StatusOK)

		// Verify we got an array with 2 users (alice and bob from test setup)
		var users []map[string]interface{}
		err := json.Unmarshal(res.BodyBytes, &users)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, len(users), 2)
	})

	t.Run("returns empty array when no users exist", func(t *testing.T) {
		app := newTestApplication(t)

		// Delete all users to test empty array response
		_, err := app.db.ExecContext(context.Background(), "DELETE FROM users")
		if err != nil {
			t.Fatal(err)
		}

		req := newTestRequest(t, http.MethodGet, "/users", nil)
		res := send(t, req, app.routes())

		assert.Equal(t, res.StatusCode, http.StatusOK)

		// Verify we got an empty array, not null
		var users []map[string]interface{}
		err = json.Unmarshal(res.BodyBytes, &users)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, len(users), 0)
	})

	t.Run("returns users in descending order by ID", func(t *testing.T) {
		app := newTestApplication(t)

		// Clear existing users and insert in specific order
		_, err := app.db.ExecContext(context.Background(), "DELETE FROM users")
		if err != nil {
			t.Fatal(err)
		}

		// Insert bob first, then alice
		id1, err := app.db.InsertUser(testUsers["bob"].email, testUsers["bob"].hashedPassword)
		if err != nil {
			t.Fatal(err)
		}
		id2, err := app.db.InsertUser(testUsers["alice"].email, testUsers["alice"].hashedPassword)
		if err != nil {
			t.Fatal(err)
		}

		req := newTestRequest(t, http.MethodGet, "/users", nil)
		res := send(t, req, app.routes())

		assert.Equal(t, res.StatusCode, http.StatusOK)

		var users []map[string]interface{}
		err = json.Unmarshal(res.BodyBytes, &users)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, len(users), 2)

		// Verify ordering: alice should be first (created second, so higher ID)
		firstUser := users[0]
		assert.Equal(t, firstUser["email"].(string), testUsers["alice"].email)
		assert.Equal(t, int64(firstUser["id"].(float64)), int64(id2))

		secondUser := users[1]
		assert.Equal(t, secondUser["email"].(string), testUsers["bob"].email)
		assert.Equal(t, int64(secondUser["id"].(float64)), int64(id1))
	})
}
