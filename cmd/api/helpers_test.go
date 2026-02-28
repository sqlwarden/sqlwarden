package main

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sqlwarden/internal/assert"

	"github.com/pascaldekloe/jwt"
)

func TestNewAuthenticationToken(t *testing.T) {
	app := newTestApplication(t)

	t.Run("generates valid JWT token and expiry time", func(t *testing.T) {
		var userID int64 = 123

		token, expiry, err := app.newAuthenticationToken(userID)
		assert.Nil(t, err)
		assert.MatchesRegexp(t, token, `^eyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$`)
		assert.True(t, expiry.After(time.Now()))
	})

	t.Run("token contains correct claims", func(t *testing.T) {
		var userID int64 = 456
		token, _, err := app.newAuthenticationToken(userID)
		assert.Nil(t, err)

		claims, err := jwt.HMACCheck([]byte(token), []byte(app.config.jwt.secretKey))
		assert.Nil(t, err)
		assert.Equal(t, claims.Subject, strconv.FormatInt(userID, 10))
		assert.Equal(t, claims.Issuer, app.config.baseURL)
		assert.Equal(t, len(claims.Audiences), 1)
		assert.Equal(t, claims.Audiences[0], app.config.baseURL)

		assert.True(t, time.Since(claims.Issued.Time()) < time.Second)
		assert.True(t, time.Since(claims.NotBefore.Time()) < time.Second)

		duration := time.Until(claims.Expires.Time())
		assert.False(t, duration < 23*time.Hour+59*time.Minute || duration > 24*time.Hour+1*time.Minute)
	})

	t.Run("generates different tokens on subsequent calls", func(t *testing.T) {
		var userID int64 = 100
		token1, _, err := app.newAuthenticationToken(userID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		time.Sleep(10 * time.Millisecond)

		token2, _, err := app.newAuthenticationToken(userID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if token1 == token2 {
			t.Error("expected different tokens for subsequent calls (different issued times)")
		}
	})
}

func TestBackgroundTask(t *testing.T) {
	t.Run("Background task runs with no errors", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, nil))

		app := newTestApplication(t)
		app.logger = logger

		req := httptest.NewRequest("GET", "/test", nil)

		executed := false
		fn := func() error {
			executed = true
			return nil
		}

		app.backgroundTask(req, fn)
		app.wg.Wait()

		assert.True(t, executed)
		assert.True(t, len(buf.String()) == 0)
	})

	t.Run("Error in background task", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, nil))

		app := newTestApplication(t)
		app.logger = logger

		req := httptest.NewRequest("GET", "/test", nil)

		executed := false
		fn := func() error {
			executed = true
			return errors.New("this is a test error")
		}

		app.backgroundTask(req, fn)
		app.wg.Wait()

		assert.True(t, executed)
		assert.True(t, strings.Contains(buf.String(), "level=ERROR"))
		assert.True(t, strings.Contains(buf.String(), `msg="this is a test error"`))
		assert.True(t, strings.Contains(buf.String(), "request.method=GET"))
		assert.True(t, strings.Contains(buf.String(), "request.url=/test"))
	})

	t.Run("Panic in background task", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, nil))

		app := newTestApplication(t)
		app.logger = logger

		req := httptest.NewRequest("GET", "/test", nil)

		executed := false
		fn := func() error {
			executed = true
			panic("this is a test error")
		}

		app.backgroundTask(req, fn)
		app.wg.Wait()

		assert.True(t, executed)
		assert.True(t, strings.Contains(buf.String(), "level=ERROR"))
		assert.True(t, strings.Contains(buf.String(), `msg="this is a test error"`))
		assert.True(t, strings.Contains(buf.String(), "request.method=GET"))
		assert.True(t, strings.Contains(buf.String(), "request.url=/test"))
	})
}
