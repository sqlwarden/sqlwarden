package web

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
	"sync/atomic"
	"testing"
	"time"

	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/smtp"
	"github.com/sqlwarden/internal/token"

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

	app.config.JWT.SecretKey = "k7mp29rf4qxhwn8vbtaj6pgucmve53y9"
	app.config.JWT.AccessTokenTTL = 24 * time.Hour
	app.config.BaseURL = "https://www.example.com"
	app.config.PersonalSpacesEnabled = true

	app.logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	app.db = newTestDB(t)
	app.mailer = smtp.NewMockMailer("test@example.com")

	return app
}

func seedAccount(t *testing.T, app *application, email, name string) database.Account {
	t.Helper()

	account, err := app.db.InsertAccount(context.Background(), email, name, nil)
	if err != nil {
		t.Fatal(err)
	}
	return account
}

func seedAccountWithToken(t *testing.T, app *application, email, name string) (database.Account, string) {
	t.Helper()

	account := seedAccount(t, app, email, name)
	tok, _, err := token.Issue(strconv.FormatInt(account.ID, 10), account.Email, account.Name, app.config.JWT.SecretKey)
	if err != nil {
		t.Fatal(err)
	}
	return account, tok
}

func createRoleForTest(t *testing.T, app *application, orgID int64, workspaceID *int64, scopeType string, permissions ...string) int64 {
	t.Helper()

	name := fmt.Sprintf("test-role-%s-%d", strings.ReplaceAll(scopeType, ":", "-"), atomic.AddUint64(&pgTestDBCounter, 1))
	roleID, err := app.enforcer.CreateRole(context.Background(), orgID, workspaceID, name, name+" description", scopeType, permissions)
	if err != nil {
		t.Fatal(err)
	}
	return roleID
}

func seedInstanceAdminAccount(t *testing.T, app *application, email, name string) (database.Account, string) {
	t.Helper()

	account, tok := seedAccountWithToken(t, app, email, name)
	if err := app.db.InsertInstanceAdmin(context.Background(), account.ID); err != nil {
		t.Fatal(err)
	}
	return account, tok
}

func seedOrganizationForAccount(t *testing.T, app *application, account database.Account, orgName string) database.Organization {
	t.Helper()

	org, err := app.db.InsertOrg(context.Background(), slugify(orgName), orgName)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.db.AddOrgMember(context.Background(), org.ID, account.ID); err != nil {
		t.Fatal(err)
	}
	if err := app.enforcer.SeedOrg(context.Background(), org.ID, account.ID); err != nil {
		t.Fatal(err)
	}
	return org
}

func seedWorkspaceForAccount(t *testing.T, app *application, org database.Organization, account database.Account, name, description string) database.Workspace {
	t.Helper()

	ws, err := app.db.InsertWorkspace(context.Background(), &org.ID, "org", org.ID, name, description)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.enforcer.SeedWorkspace(context.Background(), org.ID, ws.ID, account.ID); err != nil {
		t.Fatal(err)
	}
	return ws
}

func seedOrgOwner(t *testing.T, app *application, email, name, orgName string) (database.Account, string, database.Organization) {
	t.Helper()

	account, tok := seedInstanceAdminAccount(t, app, email, name)
	org := seedOrganizationForAccount(t, app, account, orgName)
	return account, tok, org
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
			dsn = pgTestDSN
		}
	}

	clonedDB := false
	if driver == "postgres" {
		if dsn == pgTestDSN {
			dbName := fmt.Sprintf("cmd_api_test_%d", atomic.AddUint64(&pgTestDBCounter, 1))
			pgTemplateCloneMu.Lock()
			_, err := pgAdminDB.ExecContext(context.Background(),
				fmt.Sprintf("CREATE DATABASE %s TEMPLATE %s", dbName, pgTemplateDBName))
			pgTemplateCloneMu.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			dsn = trimPostgresScheme(dsnWithDatabase("postgres://"+pgAdminDSN, dbName))
			clonedDB = true
		}
	}

	db, err := database.New(driver, dsn, slog.New(slog.NewTextHandler(io.Discard, nil)), false)
	if err != nil {
		t.Fatal(err)
	}

	if driver == "postgres" {
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(0)
	}

	t.Cleanup(func() {
		db.Close()

		switch driver {
		case "postgres":
			if clonedDB {
				dbName := databaseNameFromDSN("postgres://" + dsn)
				pgTemplateCloneMu.Lock()
				defer pgTemplateCloneMu.Unlock()
				_, _ = pgAdminDB.ExecContext(context.Background(),
					"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()",
					dbName)
				_, err = pgAdminDB.ExecContext(context.Background(), fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName))
				if err != nil {
					t.Fatal(err)
				}
			}
		case "sqlite":
			os.Remove(dsn)
		}
	})

	if driver == "sqlite" {
		err = db.MigrateUp()
		if err != nil {
			t.Fatal(err)
		}

		for _, user := range testUsers {
			acc, err := db.InsertAccount(context.Background(), user.email, user.email, &user.hashedPassword)
			if err != nil {
				t.Fatal(err)
			}
			user.id = acc.ID
		}
	}

	if driver == "postgres" && !clonedDB {
		err = db.MigrateUp()
		if err != nil {
			t.Fatal(err)
		}

		for _, user := range testUsers {
			acc, err := db.InsertAccount(context.Background(), user.email, user.email, &user.hashedPassword)
			if err != nil {
				t.Fatal(err)
			}
			user.id = acc.ID
		}
	}

	return db
}

func databaseNameFromDSN(connStr string) string {
	idx := strings.LastIndex(connStr, "/")
	if idx == -1 {
		return ""
	}
	rest := connStr[idx+1:]
	if q := strings.Index(rest, "?"); q != -1 {
		return rest[:q]
	}
	return rest
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
		json.Unmarshal(resBody, &fields)
		// err := json.Unmarshal(resBody, &fields)
		// if err != nil {
		// 	// Not a JSON object, might be an array or other type
		// 	// Don't fail here, let the test handle it
		// }
	}

	return testResponse{
		Response:   res,
		BodyFields: fields,
		BodyBytes:  resBody,
	}
}

func decodeJSONResponse(t *testing.T, body []byte, dst any) {
	t.Helper()

	if err := json.Unmarshal(body, dst); err != nil {
		t.Fatal(err)
	}
}

func assertBodyContainsJSONKeys(t *testing.T, body []byte, keys ...string) {
	t.Helper()

	var payload map[string]any
	decodeJSONResponse(t, body, &payload)
	for _, key := range keys {
		if _, ok := payload[key]; !ok {
			t.Fatalf("expected response to contain key %q", key)
		}
	}
}

func assertValidationField(t *testing.T, res testResponse, field string) {
	t.Helper()

	errorsValue, ok := res.BodyFields["field_errors"].(map[string]any)
	if !ok {
		t.Fatalf("expected field_errors in response, got %#v", res.BodyFields)
	}
	if _, ok := errorsValue[field]; !ok {
		t.Fatalf("expected validation field %q in %#v", field, errorsValue)
	}
}

func setupWorkspaceOwner(t *testing.T) (*application, database.Organization, database.Workspace, string) {
	t.Helper()

	app := newTestApp(t)
	owner, token, org := seedOrgOwner(t, app, uniqueEmail(t, "workspace-owner"), "Workspace Owner", "Acme")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Primary Workspace", "")
	return app, org, ws, token
}

func seedEnvironment(t *testing.T, app *application, workspaceID int64, orgID int64, name string) database.Environment {
	t.Helper()

	env, err := app.db.InsertEnvironment(context.Background(), workspaceID, name, "")
	if err != nil {
		t.Fatal(err)
	}
	return env
}

func seedConnection(t *testing.T, app *application, workspaceID int64, environmentID *int64, orgID int64, driver, name, accessMode string) database.Connection {
	t.Helper()

	conn, err := app.db.InsertConnection(context.Background(), workspaceID, environmentID, name, driver, "dsn", accessMode)
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

func defaultEnvironmentID(t *testing.T, app *application, workspaceID int64) int64 {
	t.Helper()

	envID, err := app.db.DefaultEnvironmentID(context.Background(), workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	return envID
}

func orgEnvConnectionsURL(orgSlug string, workspaceID, environmentID int64) string {
	return fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/environments/%d/connections", orgSlug, workspaceID, environmentID)
}

func orgConnectionURL(orgSlug string, workspaceID, environmentID int64, connectionID string) string {
	return fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/environments/%d/connections/%s", orgSlug, workspaceID, environmentID, connectionID)
}

func meEnvConnectionsURL(workspaceID, environmentID string) string {
	return fmt.Sprintf("/api/v1/me/workspaces/%s/environments/%s/connections", workspaceID, environmentID)
}

func newOrgJSONRequest(t *testing.T, method, path, body, token string) *http.Request {
	t.Helper()

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func newOrgRequest(t *testing.T, method, path, token string) *http.Request {
	t.Helper()

	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

func uniqueEmail(t *testing.T, prefix string) string {
	t.Helper()
	return fmt.Sprintf("%s-%d@example.com", prefix, time.Now().UnixNano())
}
