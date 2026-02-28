package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/smtp"

	"github.com/pascaldekloe/jwt"
)

type testUser struct {
	id             int64
	email          string
	password       string
	hashedPassword string
}

var testUsers = map[string]*testUser{
	"alice": {email: "alice@example.com", password: "testPass123!", hashedPassword: "$2a$04$27fHaQw5jwiMKYoxhLek4uyj9zp29lxtmLWGuC0MR6tuispXJn9US"},
	"bob":   {email: "bob@example.com", password: "mySecure456#", hashedPassword: "$2a$04$O6QOPBSFw14SyLBXs64MJuQd8o7GaBKYvbDqeHGgZRi6FN87aXDWC"},
}

func newTestClaims() jwt.Claims {
	var c jwt.Claims
	c.Subject = strconv.FormatInt(testUsers["alice"].id, 10)
	c.Issued = jwt.NewNumericTime(time.Now())
	c.NotBefore = jwt.NewNumericTime(time.Now())
	c.Expires = jwt.NewNumericTime(time.Now().Add(24 * time.Hour))
	c.Issuer = "https://www.example.com"
	c.Audiences = []string{"https://www.example.com"}
	return c
}

func newTestApplication(t *testing.T) *application {
	app := new(application)

	app.config.jwt.secretKey = "k7mp29rf4qxhwn8vbtaj6pgucmve53y9"
	app.config.baseURL = "https://www.example.com"

	app.logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	app.db = newTestDB(t)
	app.mailer = smtp.NewMockMailer("test@example.com")

	return app
}

func newTestDB(t *testing.T) *database.DB {
	t.Helper()

	driver := os.Getenv("TEST_DB_DRIVER")
	dsn := os.Getenv("TEST_DB_DSN")

	if driver == "" {
		driver = "postgres"
	}

	if dsn == "" {
		if driver == "sqlite" {
			dsn = "test.db"
		} else {
			dsn = "user:pass@localhost:5432/db?sslmode=disable"
		}
	}

	var schemaName string
	if driver == "postgres" {
		schemaName = fmt.Sprintf("test_schema_%d", time.Now().UnixNano())
		separator := "?"
		if strings.Contains(dsn, "?") {
			separator = "&"
		}
		dsn = fmt.Sprintf("%s%ssearch_path=%s", dsn, separator, schemaName)
	}

	db, err := database.New(driver, dsn, slog.New(slog.NewTextHandler(io.Discard, nil)), false)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		defer db.Close()

		switch driver {
		case "postgres":
			_, err = db.ExecContext(context.Background(), fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schemaName))
			if err != nil {
				t.Fatal(err)
			}
		case "sqlite":
			os.Remove(dsn)
		}
	})

	if driver == "postgres" {
		_, err = db.ExecContext(context.Background(), fmt.Sprintf("CREATE SCHEMA %s", schemaName))
		if err != nil {
			t.Fatal(err)
		}
	}

	err = db.MigrateUp()
	if err != nil {
		t.Fatal(err)
	}

	for _, user := range testUsers {
		id, err := db.InsertUser(user.email, user.hashedPassword)
		if err != nil {
			t.Fatal(err)
		}

		user.id = int64(id)
	}

	return db
}

func newTestRequest(t *testing.T, method, path string, data map[string]any) *http.Request {
	if data == nil {
		req, err := http.NewRequest(method, path, nil)
		if err != nil {
			t.Fatal(err)
		}
		return req
	}

	js, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest(method, path, bytes.NewBuffer(js))
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Content-Type", "application/json")
	return req
}

type testResponse struct {
	*http.Response
	BodyFields map[string]any
	BodyBytes  []byte
}

func send(t *testing.T, req *http.Request, h http.Handler) testResponse {
	if len(req.PostForm) > 0 {
		body := req.PostForm.Encode()
		req.Body = io.NopCloser(strings.NewReader(body))
		req.ContentLength = int64(len(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	res := rec.Result()

	defer res.Body.Close()
	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}

	fields := map[string]any{}

	if len(resBody) > 0 {
		err := json.Unmarshal(resBody, &fields)
		if err != nil {
			// Not a JSON object, might be an array or other type
			// Don't fail here, let the test handle it
		}
	}

	return testResponse{
		Response:   res,
		BodyFields: fields,
		BodyBytes:  resBody,
	}
}
