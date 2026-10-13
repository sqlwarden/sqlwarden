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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/credentials"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/encrypt"
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/execution"
	"github.com/sqlwarden/internal/identity"
	"github.com/sqlwarden/internal/orgs"
	"github.com/sqlwarden/internal/platform/clientip"
	"github.com/sqlwarden/internal/profile/server"
	"github.com/sqlwarden/internal/services"
	"github.com/sqlwarden/internal/settings"
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

// testAppOptions replaces collaborators before the service graph is built, so
// the services observe the same overrides as the handlers.
type testAppOptions struct {
	policy     access.PolicyEvaluator
	runtime    func(execution.Runtime) execution.Runtime
	revoker    execution.Revoker
	audit      audit.Writer
	logger     *slog.Logger
	fileStores func(FileStores) FileStores
	configure  func(*config.Config)
	signIn     identity.SignInStrategy
	// setup defaults to identity.FormSetup.
	setup identity.SetupStrategy
}

func newTestApplication(t *testing.T) *application {
	return newTestApplicationWith(t, testAppOptions{})
}

func newTestApplicationWith(t *testing.T, opts testAppOptions) *application {
	app := new(application)
	app.config = config.Default()
	app.invitationPolicy = orgs.InvitationsEnabled

	app.config.JWT.SecretKey = "k7mp29rf4qxhwn8vbtaj6pgucmve53y9"
	app.config.BootstrapBaseURL = "https://www.example.com"
	app.config.Files.StorageMode = config.FilesStorageModeObject
	app.config.Files.ActiveStorageBackend = config.DefaultFilesActiveBackend
	app.config.Files.StorageBackends = map[string]config.FileStorageBackend{
		config.DefaultFilesActiveBackend: {
			Type:    config.FilesStorageBackendFilesystem,
			RootDir: t.TempDir(),
		},
	}
	app.logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	state := &testAppState{db: newTestDB(t)}
	testApps.Store(app, state)
	t.Cleanup(func() { testApps.Delete(app) })
	app.audit = audit.NewCoreWriter(audit.NewSQLStore(testDB(app).DB), time.Now)
	app.clientIPs = clientip.New(nil)
	instance, found, err := testDB(app).GetInstanceSettings(context.Background())
	if err != nil || !found {
		t.Fatalf("get instance settings: found=%v err=%v", found, err)
	}
	instance.BaseURL = app.config.BootstrapBaseURL
	instance.FileRevisionsEnabled = false
	instance.SQLiteInMemoryTargetsEnabled = true
	instance, err = testDB(app).UpsertInstanceSettings(context.Background(), instance)
	if err != nil {
		t.Fatal(err)
	}
	app.initialSettings = settings.Instance(instance)
	keyring, err := encrypt.NewKeyring("test-encryption-key-32bytes!!!!!")
	if err != nil {
		t.Fatal(err)
	}
	state.keyring = keyring
	app.fileStores, err = newTestFileStores(app.config)
	if err != nil {
		t.Fatal(err)
	}
	installTestRuntime(t, app)
	enforcer, err := access.New(testDB(app).DB)
	if err != nil {
		t.Fatal(err)
	}
	app.enforcer = enforcer
	app.policy = enforcer

	if opts.configure != nil {
		opts.configure(&app.config)
	}
	if opts.policy != nil {
		app.policy = opts.policy
	}
	if opts.runtime != nil {
		app.runtime = opts.runtime(app.runtime)
		app.revoker = app.runtime
	}
	if opts.revoker != nil {
		app.revoker = opts.revoker
	}
	if opts.audit != nil {
		app.audit = opts.audit
	}
	if opts.logger != nil {
		app.logger = opts.logger
	}
	if opts.fileStores != nil {
		app.fileStores = opts.fileStores(app.fileStores)
	}
	signIn := identity.PasswordSignIn(testDB(app))
	if opts.signIn != nil {
		signIn = opts.signIn
	}
	setup := identity.FormSetup
	if opts.setup != nil {
		setup = opts.setup
	}
	buildTestServices(t, app, signIn, setup)
	app.authChain = app.newAuthChain()
	app.services.Mailer.Set(smtp.NewMockMailer("test@example.com"))

	return app
}

func buildTestServices(t *testing.T, app *application, signIn identity.SignInStrategy, setup identity.SetupStrategy) {
	t.Helper()
	core := credentials.NewEncryptedColumnProvider(testDB(app), credentials.NewKeyringSealer(testKeyring(app)), engine.ConnectionSpecFor)
	svc, err := services.Build(services.Deps{
		Config:           app.config,
		DB:               testDB(app),
		Enforcer:         app.enforcer,
		Policy:           app.policy,
		Runtime:          app.runtime,
		Revoker:          app.revoker,
		Audit:            app.audit,
		Logger:           app.logger,
		FileStores:       app.fileStores,
		Credentials:      core,
		CredentialWriter: core,
		RevealPolicy:     server.New(testDB(app)).RevealPolicy(),
		SecretSealer:     testKeyring(app),
		Keyring:          testKeyring(app),
		Invitations:      app.invitationPolicy,
		SignIn:           signIn,
		Setup:            setup,
	})
	if err != nil {
		t.Fatal(err)
	}
	svc.Settings.Subscribe(app.queueRuntimeOperations)
	app.services = svc
	app.jobStore = svc.JobStore
	app.credentials = core
}

// insertStructuredConnection stores a connection in structured form, the only
// shape runtime sessions open from.
func insertStructuredConnection(t *testing.T, app *application, workspaceID int64, environmentID *int64, name, driver string, params map[string]any) database.Connection {
	t.Helper()

	conn, err := testDB(app).InsertConnection(context.Background(), workspaceID, environmentID, name, driver, "", "open")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	if err := testDB(app).UpdateConnectionStructured(context.Background(), conn.ID, encoded, nil, nil); err != nil {
		t.Fatal(err)
	}
	conn.Params = encoded
	return conn
}

// testAppState holds the metadata database and keyring a test application was
// built on. The application itself never holds them: handlers reach metadata
// and secrets only through services, so fixtures keep them on the side.
type testAppState struct {
	db      *database.DB
	keyring *encrypt.Keyring
}

var testApps sync.Map

func testState(app *application) *testAppState {
	state, ok := testApps.Load(app)
	if !ok {
		panic("test application was not built by newTestApplicationWith")
	}
	return state.(*testAppState)
}

// testDB returns the metadata database fixtures seed and inspect directly.
func testDB(app *application) *database.DB {
	return testState(app).db
}

// testKeyring returns the keyring the application's services were built with,
// or the one a key rotation test switched to.
func testKeyring(app *application) *encrypt.Keyring {
	return testState(app).keyring
}

type testRuntimeBackends struct {
	manager *connection.Manager
	cursors *connection.QueryCursorManager
}

// testRuntimes maps each test application to the session and cursor managers
// its runtime runs on, so tests can seed sessions over fake drivers.
var testRuntimes sync.Map

func installTestRuntime(t *testing.T, app *application) {
	t.Helper()
	backends := testRuntimeBackends{
		manager: connection.New(30 * time.Minute),
		cursors: connection.NewQueryCursorManager(30 * time.Minute),
	}
	policy := settings.NewTargetPolicy(testDB(app))
	runtime := execution.NewLocal(execution.LocalConfig{
		Credentials: credentials.NewEncryptedColumnProvider(testDB(app), credentials.NewKeyringSealer(testKeyring(app)), engine.ConnectionSpecFor),
		Policy:      policy,
		Logger:      app.logger,
		Manager:     backends.manager,
		Cursors:     backends.cursors,
	})
	app.runtime = runtime
	app.revoker = runtime
	testRuntimes.Store(app, backends)
	t.Cleanup(func() {
		runtime.Shutdown()
		testRuntimes.Delete(app)
	})
}

func testBackends(t *testing.T, app *application) testRuntimeBackends {
	t.Helper()
	backends, ok := testRuntimes.Load(app)
	if !ok {
		t.Fatal("test application has no runtime installed")
	}
	return backends.(testRuntimeBackends)
}

func testConnManager(t *testing.T, app *application) *connection.Manager {
	t.Helper()
	return testBackends(t, app).manager
}

func testQueryCursorManager(t *testing.T, app *application) *connection.QueryCursorManager {
	t.Helper()
	return testBackends(t, app).cursors
}

func updateInstanceSettingsForTest(t *testing.T, app *application, mutate func(*database.InstanceSettings)) {
	t.Helper()
	settings, found, err := testDB(app).GetInstanceSettings(context.Background())
	if err != nil || !found {
		t.Fatalf("get instance settings: found=%v err=%v", found, err)
	}
	mutate(&settings)
	if _, err := testDB(app).UpsertInstanceSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
}

func inviteAndAcceptExistingAccount(t *testing.T, app *application, orgSlug, email, inviterToken, inviteeToken string) {
	t.Helper()
	created := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs/"+orgSlug+"/invitations", map[string]any{
		"email": email,
	}, inviterToken), app.routes())
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("create invitation: got status %d, body %s", created.StatusCode, created.BodyBytes)
	}
	inviteURL, _ := created.BodyFields["invite_url"].(string)
	tokenValue := inviteURL[strings.LastIndex(inviteURL, "/")+1:]
	accepted := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/invitations/"+tokenValue+"/accept", map[string]any{}, inviteeToken), app.routes())
	if accepted.StatusCode != http.StatusOK {
		t.Fatalf("accept invitation: got status %d, body %s", accepted.StatusCode, accepted.BodyBytes)
	}
}

func addOrgMemberDirect(t *testing.T, app *application, orgSlug, email string) {
	t.Helper()
	org, found, err := testDB(app).GetOrgBySlug(context.Background(), orgSlug)
	if err != nil || !found {
		t.Fatalf("get organization %q: found=%v err=%v", orgSlug, found, err)
	}
	account, found, err := testDB(app).GetAccountByEmail(context.Background(), email)
	if err != nil || !found {
		t.Fatalf("get account %q: found=%v err=%v", email, found, err)
	}
	if err := testDB(app).AddOrgMember(context.Background(), org.ID, account.ID); err != nil {
		t.Fatal(err)
	}
	if app.enforcer != nil {
		app.enforcer.InvalidatePrincipals(org.ID, account.ID)
	}
}

func seedAccount(t *testing.T, app *application, email, name string) database.Account {
	t.Helper()

	account, err := testDB(app).InsertAccount(context.Background(), email, name, nil)
	if err != nil {
		t.Fatal(err)
	}
	return account
}

func seedAccountWithToken(t *testing.T, app *application, email, name string) (database.Account, string) {
	t.Helper()

	account := seedAccount(t, app, email, name)
	authSession, err := testDB(app).InsertAuthSession(context.Background(), account.ID, time.Now().Add(7*24*time.Hour), "test-agent", "127.0.0.1", database.SessionAuth{Method: "password", Assurance: "aal1"})
	if err != nil {
		t.Fatal(err)
	}
	tok, _, err := token.IssueWithSessionTTL(strconv.FormatInt(account.ID, 10), authSession.ID, account.Email, account.Name, app.config.JWT.SecretKey, time.Duration(database.DefaultJWTAccessTokenTTLSeconds)*time.Second)
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
	if err := testDB(app).InsertInstanceAdmin(context.Background(), account.ID); err != nil {
		t.Fatal(err)
	}
	return account, tok
}

func seedOrganizationForAccount(t *testing.T, app *application, account database.Account, orgName string) database.Organization {
	t.Helper()

	org, err := testDB(app).InsertOrg(context.Background(), orgs.Slugify(orgName), orgName)
	if err != nil {
		t.Fatal(err)
	}
	if err := testDB(app).AddOrgMember(context.Background(), org.ID, account.ID); err != nil {
		t.Fatal(err)
	}
	if err := app.enforcer.SeedOrg(context.Background(), org.ID, account.ID); err != nil {
		t.Fatal(err)
	}
	return org
}

func seedWorkspaceForAccount(t *testing.T, app *application, org database.Organization, account database.Account, name, description string) database.Workspace {
	t.Helper()

	ws, err := testDB(app).InsertWorkspace(context.Background(), &org.ID, "org", org.ID, name, description)
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

	db, err := database.New(driver, dsn, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}

	if driver == "postgres" {
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
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

func (r testResponse) errorField(name string) string {
	errorValue, _ := r.BodyFields["error"].(map[string]any)
	value, _ := errorValue[name].(string)
	return value
}

func (r testResponse) ErrorCode() string { return r.errorField("code") }

func (r testResponse) ErrorReason() string { return r.errorField("reason") }

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

	errorValue, ok := res.BodyFields["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error envelope in response, got %#v", res.BodyFields)
	}
	errorsValue, ok := errorValue["field_errors"].(map[string]any)
	if !ok {
		t.Fatalf("expected field_errors in response, got %#v", res.BodyFields)
	}
	if _, ok := errorsValue[field]; !ok {
		t.Fatalf("expected validation field %q in %#v", field, errorsValue)
	}
}

func assertAPIError(t *testing.T, res testResponse, code, message string) {
	t.Helper()

	errorValue, ok := res.BodyFields["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error envelope in response, got %#v", res.BodyFields)
	}
	if code != "" {
		if got := errorValue["code"]; got != code {
			t.Fatalf("error.code = %#v, want %q", got, code)
		}
	}
	if message != "" {
		if got := errorValue["message"]; got != message {
			t.Fatalf("error.message = %#v, want %q", got, message)
		}
	}
}

func apiErrorDetails(t *testing.T, res testResponse) map[string]any {
	t.Helper()

	errorValue, ok := res.BodyFields["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error envelope in response, got %#v", res.BodyFields)
	}
	details, ok := errorValue["details"].(map[string]any)
	if !ok {
		t.Fatalf("expected error details in response, got %#v", errorValue)
	}
	return details
}

func setupWorkspaceOwner(t *testing.T) (*application, database.Organization, database.Workspace, string) {
	t.Helper()

	return setupWorkspaceOwnerWith(t, testAppOptions{})
}

func setupWorkspaceOwnerWith(t *testing.T, opts testAppOptions) (*application, database.Organization, database.Workspace, string) {
	t.Helper()

	app := newTestAppWith(t, opts)
	owner, token, org := seedOrgOwner(t, app, uniqueEmail(t, "workspace-owner"), "Workspace Owner", "Acme")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Primary Workspace", "")
	return app, org, ws, token
}

func seedEnvironment(t *testing.T, app *application, workspaceID int64, orgID int64, name string) database.Environment {
	t.Helper()

	env, err := testDB(app).InsertEnvironment(context.Background(), workspaceID, name, "")
	if err != nil {
		t.Fatal(err)
	}
	return env
}

func seedConnection(t *testing.T, app *application, workspaceID int64, environmentID *int64, orgID int64, driver, name, accessMode string) database.Connection {
	t.Helper()

	conn, err := testDB(app).InsertConnection(context.Background(), workspaceID, environmentID, name, driver, "dsn", accessMode)
	if err != nil {
		t.Fatal(err)
	}
	if spec, ok := engine.ConnectionSpecFor(driver); ok {
		for _, field := range spec.Fields() {
			var params string
			switch field.Key {
			case "dsn":
				params = `{"dsn":"dsn"}`
			case "path":
				params = `{"path":":memory:"}`
			default:
				continue
			}
			if err := testDB(app).UpdateConnectionStructured(context.Background(), conn.ID, json.RawMessage(params), nil, nil); err != nil {
				t.Fatal(err)
			}
			conn.Params = json.RawMessage(params)
			break
		}
	}
	return conn
}

func defaultEnvironmentID(t *testing.T, app *application, workspaceID int64) int64 {
	t.Helper()

	envID, err := testDB(app).DefaultEnvironmentID(context.Background(), workspaceID)
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
