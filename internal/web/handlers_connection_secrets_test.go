package web

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/audit"
	"golang.org/x/crypto/ssh"
)

func createPostgresConnectionWithPassword(t *testing.T, app *application, orgSlug string, wsID, envID int64, tok, password string) string {
	t.Helper()
	res := send(t, newAuthRequest(t, http.MethodPost, orgEnvConnectionsURL(orgSlug, wsID, envID), map[string]any{
		"name":    "Primary",
		"driver":  "postgres",
		"params":  map[string]any{"host": "localhost", "port": "5432", "database": "test", "username": "test"},
		"secrets": map[string]any{"password": password},
	}, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusCreated)
	return fmt.Sprintf("%v", res.BodyFields["id"])
}

func TestConnectionResponsesReportSecretStateWithoutValues(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, "conn-secret-shape@example.com", "Shape Owner", "Shape Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Shape WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)

	connID := createPostgresConnectionWithPassword(t, app, org.Slug, ws.ID, envID, tok, "shape-secret-value")
	res := send(t, newAuthRequest(t, http.MethodGet, orgConnectionURL(org.Slug, ws.ID, envID, connID), nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.False(t, strings.Contains(string(res.BodyBytes), "shape-secret-value"))

	secrets, ok := res.BodyFields["secrets"].(map[string]any)
	if !ok {
		t.Fatalf("secrets = %#v, want object", res.BodyFields["secrets"])
	}
	password, ok := secrets["password"].(map[string]any)
	if !ok {
		t.Fatalf("password state = %#v", secrets["password"])
	}
	assert.Equal(t, password["set"], true)
	assert.Equal(t, password["source"], "stored")
	params, _ := res.BodyFields["params"].(map[string]any)
	assert.Equal(t, params["host"], "localhost")

	clearRes := send(t, newAuthRequest(t, http.MethodPatch, orgConnectionURL(org.Slug, ws.ID, envID, connID),
		map[string]any{"secrets": map[string]any{"password": nil}, "force": true}, tok), app.routes())
	assert.Equal(t, clearRes.StatusCode, http.StatusNoContent)
	after := send(t, newAuthRequest(t, http.MethodGet, orgConnectionURL(org.Slug, ws.ID, envID, connID), nil, tok), app.routes())
	state := after.BodyFields["secrets"].(map[string]any)["password"].(map[string]any)
	assert.Equal(t, state["set"], false)
}

func TestRevealConnectionSecretStatuses(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, "conn-reveal-status@example.com", "Status Owner", "Status Org")
	setConnectionSecretRevealForTest(t, app, org.Slug, true)
	ws := seedWorkspaceForAccount(t, app, org, owner, "Status WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	connID := createPostgresConnectionWithPassword(t, app, org.Slug, ws.ID, envID, tok, "status-secret")
	base := orgConnectionURL(org.Slug, ws.ID, envID, connID)

	unset := send(t, newAuthRequest(t, http.MethodPost, base+"/secrets/ssh_password/reveal", nil, tok), app.routes())
	assert.Equal(t, unset.StatusCode, http.StatusConflict)
	assert.Equal(t, unset.ErrorCode(), "secret_not_revealable")
	assert.Equal(t, unset.Header.Get("Cache-Control"), "no-store")

	unknownName := send(t, newAuthRequest(t, http.MethodPost, base+"/secrets/not_a_secret/reveal", nil, tok), app.routes())
	assert.Equal(t, unknownName.StatusCode, http.StatusUnprocessableEntity)
	assert.Equal(t, unknownName.Header.Get("Cache-Control"), "no-store")

	missing := send(t, newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.Itoa(999999))+"/secrets/password/reveal", nil, tok), app.routes())
	assert.Equal(t, missing.StatusCode, http.StatusNotFound)
	assert.Equal(t, missing.Header.Get("Cache-Control"), "no-store")

	_, foreignTok, foreignOrg := seedOrgOwner(t, app, "conn-reveal-foreign@example.com", "Foreign Owner", "Foreign Org")
	cross := send(t, newAuthRequest(t, http.MethodPost,
		orgConnectionURL(foreignOrg.Slug, ws.ID, envID, connID)+"/secrets/password/reveal", nil, foreignTok), app.routes())
	assert.Equal(t, cross.StatusCode, http.StatusNotFound)
	assert.False(t, strings.Contains(string(cross.BodyBytes), "status-secret"))
	assert.Equal(t, cross.Header.Get("Cache-Control"), "no-store")

	nonMember, nonMemberTok := seedAccountWithToken(t, app, "conn-reveal-nonmember@example.com", "Non Member")
	_ = nonMember
	outsider := send(t, newAuthRequest(t, http.MethodPost, base+"/secrets/password/reveal", nil, nonMemberTok), app.routes())
	assert.Equal(t, outsider.StatusCode, http.StatusForbidden)
	assert.Equal(t, outsider.Header.Get("Cache-Control"), "no-store")
}

func TestRevealPersonalSpaceConnectionSecret(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, "conn-reveal-personal@example.com", "Personal Owner", "Personal Org")
	setConnectionSecretRevealForTest(t, app, org.Slug, true)
	ws := seedWorkspaceForAccount(t, app, org, owner, "Personal WS", "")

	base := fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/connections", org.Slug, ws.ID)
	createRes := send(t, newAuthRequest(t, http.MethodPost, base, map[string]any{
		"name": "Personal", "driver": "postgres",
		"params":  map[string]any{"host": "localhost", "port": "5432", "database": "test", "username": "test"},
		"secrets": map[string]any{"password": "personal-secret"},
	}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	res := send(t, newAuthRequest(t, http.MethodPost, base+"/"+connID+"/secrets/password/reveal", nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["value"], "personal-secret")
	assert.Equal(t, res.Header.Get("Cache-Control"), "no-store")
}

func TestConnectionParamTypeErrorNamesField(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, "conn-param-type@example.com", "Type Owner", "Type Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Type WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)

	res := send(t, newAuthRequest(t, http.MethodPost, orgEnvConnectionsURL(org.Slug, ws.ID, envID), map[string]any{
		"name": "Typed", "driver": "postgres",
		"params": map[string]any{"host": "localhost", "port": 5432},
	}, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusBadRequest)
	message, _ := res.BodyFields["error"].(map[string]any)["message"].(string)
	assert.True(t, strings.Contains(message, "params"))
}

func TestEngineConnectionFields(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok, _ := seedOrgOwner(t, app, "conn-fields@example.com", "Fields Owner", "Fields Org")

	res := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/engines/postgres/connection-fields", nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	fields, ok := res.BodyFields["fields"].([]any)
	if !ok || len(fields) == 0 {
		t.Fatalf("fields = %#v", res.BodyFields["fields"])
	}

	unknown := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/engines/db2/connection-fields", nil, tok), app.routes())
	assert.Equal(t, unknown.StatusCode, http.StatusNotFound)

	anon := send(t, newTestRequest(t, http.MethodGet, "/api/v1/engines/postgres/connection-fields", nil), app.routes())
	assert.Equal(t, anon.StatusCode, http.StatusUnauthorized)
}

func newSSHKeyPEM(t *testing.T, passphrase string) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKeyWithPassphrase(priv, "", []byte(passphrase))
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(block))
}

func TestConnectionSecretsNeverLeak(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	var logs bytes.Buffer
	app.logger = slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	owner, tok, org := seedOrgOwner(t, app, "conn-leak@example.com", "Leak Owner", "Leak Org")
	member, memberTok := seedAccountWithToken(t, app, "conn-leak-member@example.com", "Leak Member")
	if err := app.db.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	ws := seedWorkspaceForAccount(t, app, org, owner, "Leak WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	setConnectionSecretRevealForTest(t, app, org.Slug, false)

	const passphraseCanary = "zq-canary-ssh-passphrase-5d1e"
	canaries := map[string]string{
		"password":        "zq-canary-password-7f3a91",
		"ssh_password":    "zq-canary-ssh-password-2b8c40",
		"ssh_private_key": newSSHKeyPEM(t, passphraseCanary),
		"ssh_passphrase":  passphraseCanary,
		"tls_client_key":  "zq-canary-tls-client-key-9e6d13",
	}
	secretsBody := func() map[string]any {
		out := map[string]any{}
		for name, value := range canaries {
			out[name] = value
		}
		return out
	}
	params := map[string]any{"host": "127.0.0.1", "port": "1", "database": "test", "username": "test"}
	sshConfig := map[string]any{"enabled": true, "host": "127.0.0.1", "port": 1, "user": "u", "auth_method": "private_key", "insecure_skip_host_key": true}
	tlsConfig := map[string]any{"mode": "require", "client_cert_pem": "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----"}
	full := func(extra map[string]any) map[string]any {
		body := map[string]any{"params": params, "ssh_config": sshConfig, "tls_config": tlsConfig, "secrets": secretsBody()}
		for k, v := range extra {
			body[k] = v
		}
		return body
	}

	type checked struct {
		label string
		res   testResponse
	}
	var all []checked
	record := func(label string, res testResponse) testResponse {
		all = append(all, checked{label, res})
		return res
	}
	base := orgEnvConnectionsURL(org.Slug, ws.ID, envID)

	createRes := record("create", send(t, newAuthRequest(t, http.MethodPost, base, full(map[string]any{"name": "Leaky", "driver": "postgres"}), tok), app.routes()))
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])
	connURL := orgConnectionURL(org.Slug, ws.ID, envID, connID)

	record("get", send(t, newAuthRequest(t, http.MethodGet, connURL, nil, tok), app.routes()))
	record("list", send(t, newAuthRequest(t, http.MethodGet, base, nil, tok), app.routes()))
	record("update", send(t, newAuthRequest(t, http.MethodPatch, connURL, full(map[string]any{"name": "Leaky 2", "force": true}), tok), app.routes()))
	record("test", send(t, newAuthRequest(t, http.MethodPost, base+"/test", full(map[string]any{"driver": "postgres"}), tok), app.routes()))
	record("test stored", send(t, newAuthRequest(t, http.MethodPost, base+"/test", map[string]any{"driver": "postgres", "connection_id": createRes.BodyFields["id"], "params": params}, tok), app.routes()))

	invalidCreate := record("create invalid params", send(t, newAuthRequest(t, http.MethodPost, base,
		full(map[string]any{"name": "Bad", "driver": "postgres", "params": map[string]any{"bogus": "x"}}), tok), app.routes()))
	assert.Equal(t, invalidCreate.StatusCode, http.StatusUnprocessableEntity)
	invalidDriver := record("create unknown driver", send(t, newAuthRequest(t, http.MethodPost, base,
		full(map[string]any{"name": "Bad", "driver": "db2"}), tok), app.routes()))
	assert.Equal(t, invalidDriver.StatusCode, http.StatusUnprocessableEntity)
	invalidTest := record("test unknown driver", send(t, newAuthRequest(t, http.MethodPost, base+"/test", full(map[string]any{"driver": "db2"}), tok), app.routes()))
	assert.Equal(t, invalidTest.StatusCode, http.StatusUnprocessableEntity)
	wrongType := record("create wrong type", send(t, newAuthRequest(t, http.MethodPost, base,
		full(map[string]any{"name": "Bad", "driver": "postgres", "params": map[string]any{"host": 5}}), tok), app.routes()))
	assert.Equal(t, wrongType.StatusCode, http.StatusBadRequest)
	failedUpdate := record("failed update", send(t, newAuthRequest(t, http.MethodPatch, connURL,
		full(map[string]any{"params": map[string]any{"bogus": "x"}}), tok), app.routes()))
	assert.Equal(t, failedUpdate.StatusCode, http.StatusUnprocessableEntity)
	badSecretName := record("unknown secret name", send(t, newAuthRequest(t, http.MethodPatch, connURL,
		map[string]any{"secrets": map[string]any{"password": canaries["password"], "nope": canaries["ssh_password"]}}, tok), app.routes()))
	assert.Equal(t, badSecretName.StatusCode, http.StatusUnprocessableEntity)

	for name := range canaries {
		revealURL := connURL + "/secrets/" + name + "/reveal"
		disabled := record("reveal disabled "+name, send(t, newAuthRequest(t, http.MethodPost, revealURL, nil, tok), app.routes()))
		assert.Equal(t, disabled.StatusCode, http.StatusForbidden)
		assert.Equal(t, disabled.ErrorCode(), "reveal_disabled")
		assert.Equal(t, disabled.Header.Get("Cache-Control"), "no-store")
	}

	setConnectionSecretRevealForTest(t, app, org.Slug, true)
	for name := range canaries {
		denied := record("reveal denied "+name, send(t, newAuthRequest(t, http.MethodPost, connURL+"/secrets/"+name+"/reveal", nil, memberTok), app.routes()))
		assert.Equal(t, denied.StatusCode, http.StatusForbidden)
		assert.Equal(t, denied.Header.Get("Cache-Control"), "no-store")
	}

	revealed := map[string]testResponse{}
	for name := range canaries {
		res := send(t, newAuthRequest(t, http.MethodPost, connURL+"/secrets/"+name+"/reveal", nil, tok), app.routes())
		assert.Equal(t, res.StatusCode, http.StatusOK)
		assert.Equal(t, res.Header.Get("Cache-Control"), "no-store")
		assert.Equal[any](t, res.BodyFields["value"], canaries[name])
		revealed[name] = res
	}

	for _, item := range all {
		for name, canary := range canaries {
			if strings.Contains(string(item.res.BodyBytes), canary) {
				t.Errorf("%s response leaked %s", item.label, name)
			}
		}
	}
	for name, res := range revealed {
		for other, canary := range canaries {
			if other != name && strings.Contains(string(res.BodyBytes), canary) {
				t.Errorf("reveal of %s carried %s", name, other)
			}
		}
	}

	events, err := audit.NewSQLStore(app.db.DB).Events(context.Background(), 200)
	if err != nil {
		t.Fatal(err)
	}
	revealEvents := 0
	for _, event := range events {
		if event.Action == "connection.secret_revealed" {
			revealEvents++
		}
	}
	assert.Equal(t, revealEvents, len(canaries))
	auditJSON, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	auditText := string(auditJSON)
	for name, canary := range canaries {
		if strings.Contains(logs.String(), canary) {
			t.Errorf("logs leaked %s", name)
		}
		if strings.Contains(auditText, canary) {
			t.Errorf("audit events leaked %s", name)
		}
	}
}
