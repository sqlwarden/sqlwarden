package web

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/sqlwarden/internal/assert"
)

func completionIndexURL(org string, wsID, envID, connID int64) string {
	return orgConnectionURL(org, wsID, envID, strconv.FormatInt(connID, 10)) + "/schema/completion-index"
}

func completionIndexObjects(body map[string]any) []map[string]any {
	raw, _ := body["objects"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func completionIndexColumns(body map[string]any) []map[string]any {
	raw, _ := body["columns"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func hasIndexObject(objects []map[string]any, schema, name, kind string) bool {
	for _, o := range objects {
		if o["schema"] == schema && o["name"] == name && o["kind"] == kind {
			return true
		}
	}
	return false
}

func hasIndexColumn(columns []map[string]any, schema, table, name, dataType string, nullable bool) bool {
	for _, c := range columns {
		if c["schema"] == schema && c["table"] == table && c["name"] == name &&
			c["type"] == dataType && c["nullable"] == nullable {
			return true
		}
	}
	return false
}

func hasString(values any, want string) bool {
	list, _ := values.([]any)
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func TestCompletionIndexRejectsForeignSession(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, token, org := seedOrgOwner(t, app, uniqueEmail(t, "completion-index-scope"), "Index", "Index Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Index WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	target := seedConnection(t, app, ws.ID, &envID, org.ID, "postgres", "Target", "open")
	other := seedConnection(t, app, ws.ID, &envID, org.ID, "postgres", "Other", "open")
	disableSchemaSnapshots(t, app, target.ID)
	session := openSchemaSession(t, app, owner.ID, other.ID, schemaFakeDriver{})

	req := newAuthRequest(t, http.MethodGet, completionIndexURL(org.Slug, ws.ID, envID, target.ID), nil, token)
	req.Header.Set("X-Warden-Session", session.ID)
	res := send(t, req, app.routes())

	assert.Equal(t, res.StatusCode, http.StatusForbidden)
}
