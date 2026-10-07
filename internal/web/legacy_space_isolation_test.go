package web

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/sqlwarden/internal/assert"
)

func TestLegacyPersonalSpaceIsolatedFromOrgRoutes(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, orgTok, org := seedOrgOwner(t, app, "space-org-owner@example.com", "Org Owner", "Acme")

	spaceOwner, spaceTok := seedAccountWithToken(t, app, "space-owner@example.com", "Space Owner")
	addOrgMemberDirect(t, app, org.Slug, spaceOwner.Email)

	ctx := context.Background()
	ws, err := app.db.InsertWorkspace(ctx, nil, "space", spaceOwner.ID, "Legacy Space", "")
	if err != nil {
		t.Fatal(err)
	}
	env, err := app.db.InsertEnvironment(ctx, ws.ID, "legacy-env", "")
	if err != nil {
		t.Fatal(err)
	}
	conn := seedConnection(t, app, ws.ID, &env.ID, org.ID, "sqlite", "legacy-conn", "read_write")

	wsURL := fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d", org.Slug, ws.ID)
	paths := []string{
		wsURL,
		fmt.Sprintf("%s/environments/%d", wsURL, env.ID),
		orgConnectionURL(org.Slug, ws.ID, env.ID, fmt.Sprint(conn.ID)),
	}

	for name, tok := range map[string]string{"org owner": orgTok, "space owner": spaceTok} {
		for _, path := range paths {
			res := send(t, newAuthRequest(t, http.MethodGet, path, nil, tok), app.routes())
			if res.StatusCode != http.StatusNotFound {
				t.Errorf("%s: GET %s returned %d", name, path, res.StatusCode)
			}
		}

		listRes := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/orgs/"+org.Slug+"/workspaces", nil, tok), app.routes())
		assert.Equal(t, listRes.StatusCode, http.StatusOK)
		var payload struct {
			Items []map[string]any `json:"items"`
		}
		decodeJSONResponse(t, listRes.BodyBytes, &payload)
		for _, item := range payload.Items {
			if fmt.Sprint(item["id"]) == fmt.Sprint(ws.ID) {
				t.Errorf("%s: legacy space workspace listed in org workspaces", name)
			}
		}
	}
}
