package web

import (
	"bytes"
	"compress/gzip"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/assert"
)

func TestRecoverPanic(t *testing.T) {
	t.Run("Allows normal requests to proceed", func(t *testing.T) {
		app := newTestApplication(t)
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		})

		req := newTestRequest(t, http.MethodGet, "/test", nil)

		res := send(t, req, app.recoverPanic(next))
		assert.Equal(t, res.StatusCode, http.StatusTeapot)
	})

	t.Run("Recovers from panic and sends a 500 response", func(t *testing.T) {
		var buf bytes.Buffer
		app := newTestApplication(t)
		app.logger = slog.New(slog.NewTextHandler(&buf, nil))

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic("something went wrong")
		})

		req := newTestRequest(t, http.MethodGet, "/test", nil)

		res := send(t, req, app.recoverPanic(next))
		assert.Equal(t, res.StatusCode, http.StatusInternalServerError)
		assertAPIError(t, res, apiErrorInternalServer, "The server encountered a problem and could not process your request.")
	})
}

func TestNoStoreCache(t *testing.T) {
	t.Run("Sets Cache-Control no-store so the browser does not cache API responses", func(t *testing.T) {
		app := newTestApplication(t)
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusGone)
		})

		req := newTestRequest(t, http.MethodGet, "/test", nil)

		res := send(t, req, app.noStoreCache(next))
		assert.Equal(t, res.StatusCode, http.StatusGone)
		assert.Equal(t, res.Header.Get("Cache-Control"), "no-store")
	})
}

func TestRoutesCompressResponsesWhenRequested(t *testing.T) {
	app := newTestApplication(t)
	req := newTestRequest(t, http.MethodGet, "/api/setup/status", nil)
	req.Header.Set("Accept-Encoding", "gzip")

	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.Header.Get("Content-Encoding"), "gzip")
	assert.True(t, strings.Contains(res.Header.Get("Vary"), "Accept-Encoding"))

	gz, err := gzip.NewReader(bytes.NewReader(res.BodyBytes))
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()

	body, err := io.ReadAll(gz)
	if err != nil {
		t.Fatal(err)
	}
	assert.True(t, strings.Contains(string(body), "configured"))
}

func TestRoutesDoNotCompressWithoutAcceptEncoding(t *testing.T) {
	app := newTestApplication(t)
	req := newTestRequest(t, http.MethodGet, "/api/setup/status", nil)

	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.Header.Get("Content-Encoding"), "")
	assert.True(t, strings.Contains(string(res.BodyBytes), "configured"))
}

func TestLogAccess(t *testing.T) {
	t.Run("Logs the request and response details", func(t *testing.T) {
		var buf bytes.Buffer
		app := newTestApplication(t)
		app.logger = slog.New(slog.NewTextHandler(&buf, nil))

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTeapot)
			w.Write([]byte(`{"Message": "I'm a test teapot"}`))
		})

		req := newTestRequest(t, http.MethodGet, "/test", nil)

		res := send(t, req, app.logAccess(next))
		assert.Equal(t, res.StatusCode, http.StatusTeapot)
		assert.True(t, strings.Contains(buf.String(), "level=WARN"))
		assert.True(t, strings.Contains(buf.String(), `msg="http request"`))
		assert.True(t, strings.Contains(buf.String(), "request.method=GET"))
		assert.True(t, strings.Contains(buf.String(), "request.path=/test"))
		assert.True(t, strings.Contains(buf.String(), "response.status=418"))
		assert.True(t, strings.Contains(buf.String(), "response.size=32"))
	})
}
