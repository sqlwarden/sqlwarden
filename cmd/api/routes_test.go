package main

import (
	"net/http"
	"testing"

	"github.com/sqlwarden/internal/assert"
)

func TestRoutes(t *testing.T) {
	t.Run("Sends a 404 response for non-existent API routes", func(t *testing.T) {
		app := newTestApplication(t)

		req := newTestRequest(t, http.MethodGet, "/api/v1/nonexistent", nil)

		res := send(t, req, app.routes())
		assert.Equal(t, res.StatusCode, http.StatusNotFound)
		assert.Equal(t, res.BodyFields["Error"], "The requested resource could not be found")
	})

}
