package web

import (
	"net/http"
	"testing"

	"github.com/sqlwarden/internal/assert"
)

func TestPersonalSpaceRoutesAreGone(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok := seedAccountWithToken(t, app, "gone@example.com", "Gone User")

	for _, path := range []string{"/api/v1/me/workspaces", "/api/v1/me/workspaces/1/environments"} {
		res := send(t, newAuthRequest(t, http.MethodGet, path, nil, tok), app.routes())
		assert.Equal(t, res.StatusCode, http.StatusNotFound)
	}
}

func TestMeStillServedAfterPersonalSpaceRemoval(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok := seedAccountWithToken(t, app, "still@example.com", "Still User")

	res := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/me", nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
}
