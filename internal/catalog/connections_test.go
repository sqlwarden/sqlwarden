package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/credentials"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/execution"
	"github.com/sqlwarden/internal/settings"

	_ "github.com/sqlwarden/internal/engine/engines/oracle"
	_ "github.com/sqlwarden/internal/engine/engines/postgres"
)

type fakeStore struct {
	members       map[[2]int64]bool
	workspaces    map[int64]database.Workspace
	environments  map[int64]database.Environment
	connections   map[int64]database.Connection
	nextID        int64
	cacheDeletes  int
	structuredErr error
	updateErr     error
}

func (s *fakeStore) IsOrgMember(_ context.Context, orgID, accountID int64) (bool, error) {
	return s.members[[2]int64{orgID, accountID}], nil
}
func (s *fakeStore) GetWorkspace(_ context.Context, id int64) (database.Workspace, bool, error) {
	value, ok := s.workspaces[id]
	return value, ok, nil
}
func (s *fakeStore) GetEnvironment(_ context.Context, id int64) (database.Environment, bool, error) {
	value, ok := s.environments[id]
	return value, ok, nil
}
func (s *fakeStore) GetConnection(_ context.Context, id int64) (database.Connection, bool, error) {
	value, ok := s.connections[id]
	return value, ok, nil
}
func (s *fakeStore) InsertConnectionWithScope(_ context.Context, workspaceID int64, envID *int64, name, driver, _ string, accessMode string, defaultScope metadata.ScopePath, showSystemSchemas, showAllDatabases bool) (database.Connection, error) {
	s.nextID++
	environmentID := int64(20)
	if envID != nil {
		environmentID = *envID
	}
	conn := database.Connection{ID: s.nextID, WorkspaceID: workspaceID, EnvironmentID: environmentID, Name: name, Driver: driver, AccessMode: accessMode, SchemaSnapshotPolicy: database.SchemaSnapshotPolicyInherit, DefaultScope: defaultScope, ShowSystemSchemas: showSystemSchemas, ShowAllDatabases: showAllDatabases, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	s.connections[conn.ID] = conn
	return conn, nil
}
func (s *fakeStore) UpdateConnectionWithScopeAndPolicy(_ context.Context, id int64, name, accessMode, snapshotPolicy string, defaultScope metadata.ScopePath, showSystemSchemas, showAllDatabases bool) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	conn := s.connections[id]
	conn.Name, conn.AccessMode, conn.SchemaSnapshotPolicy, conn.DefaultScope = name, accessMode, snapshotPolicy, defaultScope
	conn.ShowSystemSchemas, conn.ShowAllDatabases = showSystemSchemas, showAllDatabases
	s.connections[id] = conn
	return nil
}
func (s *fakeStore) UpdateConnectionStructured(_ context.Context, id int64, params, tlsConfig, sshConfig json.RawMessage) error {
	if s.structuredErr != nil {
		return s.structuredErr
	}
	conn := s.connections[id]
	conn.Params, conn.TLSConfig, conn.SSHConfig = cloneRaw(params), cloneRaw(tlsConfig), cloneRaw(sshConfig)
	s.connections[id] = conn
	return nil
}
func (s *fakeStore) DeleteConnection(_ context.Context, id int64) error {
	delete(s.connections, id)
	return nil
}
func (s *fakeStore) DeleteSchemaCache(context.Context, int64) error {
	s.cacheDeletes++
	return nil
}

func cloneRaw(value json.RawMessage) json.RawMessage { return append(json.RawMessage(nil), value...) }

type fakePolicy struct{ permissions map[string]bool }

func (p fakePolicy) Can(_ context.Context, _, _ int64, _, _ string, _ int64, permission string) bool {
	return p.permissions[permission]
}
func (fakePolicy) EffectivePermissions(context.Context, int64, int64, string, string, int64) ([]string, error) {
	return nil, nil
}

type fakeCredentials struct {
	states      map[credentials.SecretName]credentials.SecretState
	values      map[credentials.SecretName]string
	setCalls    map[credentials.SecretName][]string
	clearCalls  map[credentials.SecretName]int
	revealCalls int
	setErr      map[credentials.SecretName]error
}

func newFakeCredentials() *fakeCredentials {
	return &fakeCredentials{states: map[credentials.SecretName]credentials.SecretState{}, values: map[credentials.SecretName]string{}, setCalls: map[credentials.SecretName][]string{}, clearCalls: map[credentials.SecretName]int{}}
}
func (f *fakeCredentials) Resolve(context.Context, credentials.ConnectionRef) (credentials.Credentials, error) {
	return credentials.Credentials{}, nil
}
func (f *fakeCredentials) Reveal(_ context.Context, _ credentials.ConnectionRef, name credentials.SecretName) (string, error) {
	f.revealCalls++
	state := f.states[name]
	if !state.Set || state.Source != credentials.SourceStored {
		return "", credentials.ErrNotRevealable
	}
	return f.values[name], nil
}
func (f *fakeCredentials) Describe(context.Context, credentials.ConnectionRef) (map[credentials.SecretName]credentials.SecretState, error) {
	states := make(map[credentials.SecretName]credentials.SecretState, len(allSecretNames))
	for _, name := range allSecretNames {
		states[name] = f.states[name]
	}
	return states, nil
}
func (f *fakeCredentials) Set(_ context.Context, _ credentials.ConnectionRef, name credentials.SecretName, value string) error {
	if err := f.setErr[name]; err != nil {
		return err
	}
	f.setCalls[name] = append(f.setCalls[name], value)
	f.states[name] = credentials.SecretState{Set: true, Source: credentials.SourceStored}
	f.values[name] = value
	return nil
}
func (f *fakeCredentials) Clear(_ context.Context, _ credentials.ConnectionRef, name credentials.SecretName) error {
	f.clearCalls[name]++
	delete(f.states, name)
	delete(f.values, name)
	return nil
}

type fakeSpec struct{}

func (fakeSpec) Fields() []engine.FieldSpec {
	return []engine.FieldSpec{{Key: "host", Label: "Host", Type: engine.FieldTypeString, Required: true}, {Key: "password", Label: "Password", Type: engine.FieldTypeString, Secret: true}}
}
func (fakeSpec) BuildDSN(params engine.Params, secrets engine.Secrets) (string, error) {
	if params["host"] == "" {
		return "", errors.New("host required")
	}
	return "fake://" + params["host"] + "/" + secrets["password"], nil
}
func (fakeSpec) ParseDSN(string) (engine.Params, engine.Secrets, error) { return nil, nil, nil }

type fakeTargetPolicy struct {
	err   error
	calls int
}

type captureProber struct {
	creds  credentials.Credentials
	probed bool
}

type fakeAncestry struct{ invalidated []int64 }

func (a *fakeAncestry) InvalidateAncestry(_ string, id int64) {
	a.invalidated = append(a.invalidated, id)
}

type fakeRevoker struct {
	active  int
	revoked int
}

func (r *fakeRevoker) CountForConnection(context.Context, string) (int, error) {
	return r.active, nil
}
func (r *fakeRevoker) RevokeConnection(context.Context, string) (int, error) {
	r.revoked++
	return r.active, nil
}

func (p *captureProber) Probe(_ context.Context, _ execution.Scope, creds credentials.Credentials, _ execution.Limits, _ func(metadata.SchemaInspector) error) error {
	p.creds, p.probed = creds, true
	return execution.ErrSchemaUnsupported
}

func (p *fakeTargetPolicy) Check(context.Context, string, string) error {
	p.calls++
	return p.err
}

func catalogFixture() (*Service, *fakeStore, *fakeCredentials, access.Principal) {
	orgID, accountID := int64(1), int64(7)
	store := &fakeStore{
		members:      map[[2]int64]bool{{orgID, accountID}: true},
		workspaces:   map[int64]database.Workspace{10: {ID: 10, OrgID: &orgID, OwnerType: "org", OwnerID: orgID}},
		environments: map[int64]database.Environment{20: {ID: 20, WorkspaceID: 10}},
		connections: map[int64]database.Connection{30: {
			ID: 30, WorkspaceID: 10, EnvironmentID: 20, Name: "primary", Driver: "fake",
			Params: json.RawMessage(`{"host":"old"}`), AccessMode: "open", SchemaSnapshotPolicy: database.SchemaSnapshotPolicyInherit,
		}},
		nextID: 30,
	}
	provider := newFakeCredentials()
	service := New(Deps{
		Store: store, Policy: fakePolicy{permissions: map[string]bool{access.PermConnRead: true, access.PermConnUpdate: true, access.PermConnCreate: true, access.PermConnDelete: true, access.PermConnRevealSecret: true}},
		Credentials: provider, Writer: provider, RevealPolicy: fakeRevealPolicy{allowed: true},
		SpecLookup: func(string) (engine.ConnectionSpec, bool) { return fakeSpec{}, true },
	})
	principal := access.Principal{Subject: access.SubjectRef{Kind: access.SubjectAccount, ID: accountID}}
	return service, store, provider, principal
}

func TestGetForeignAndUnknownConnectionAreNotFound(t *testing.T) {
	service, _, _, principal := catalogFixture()
	for _, ref := range []ConnRef{
		{OrgID: 1, WorkspaceID: 10, ConnectionID: 999},
		{OrgID: 1, WorkspaceID: 999, ConnectionID: 30},
	} {
		if _, err := service.Get(context.Background(), principal, ref); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Get(%+v) error = %v, want ErrNotFound", ref, err)
		}
	}
}

func TestUpdateSecretsKeepClearAndReplacePerName(t *testing.T) {
	for _, name := range allSecretNames {
		t.Run(string(name), func(t *testing.T) {
			service, _, provider, principal := catalogFixture()
			provider.states[name] = credentials.SecretState{Set: true, Source: credentials.SourceStored}
			provider.values[name] = "old"
			ref := ConnRef{OrgID: 1, WorkspaceID: 10, ConnectionID: 30}

			newName := "renamed"
			if err := service.Update(context.Background(), principal, ref, UpdateInput{Name: &newName}); err != nil {
				t.Fatalf("keep Update: %v", err)
			}
			if len(provider.setCalls[name]) != 0 || provider.clearCalls[name] != 0 {
				t.Fatalf("absent secret was changed: sets=%v clears=%d", provider.setCalls[name], provider.clearCalls[name])
			}

			if err := service.Update(context.Background(), principal, ref, UpdateInput{Secrets: map[credentials.SecretName]*string{name: nil}}); err != nil {
				t.Fatalf("clear Update: %v", err)
			}
			if provider.clearCalls[name] != 1 {
				t.Fatalf("clear calls = %d, want 1", provider.clearCalls[name])
			}

			replacement := "new"
			if err := service.Update(context.Background(), principal, ref, UpdateInput{Secrets: map[credentials.SecretName]*string{name: &replacement}, Force: true}); err != nil {
				t.Fatalf("replace Update: %v", err)
			}
			calls := provider.setCalls[name]
			if len(calls) != 1 || calls[0] != replacement {
				t.Fatalf("set calls = %v, want [%q]", calls, replacement)
			}
		})
	}
}

func TestCreateHonorsTargetPolicyDenial(t *testing.T) {
	service, store, _, principal := catalogFixture()
	want := errors.New("denied")
	targetPolicy := &fakeTargetPolicy{err: want}
	service.deps.TargetPolicy = targetPolicy
	_, err := service.Create(context.Background(), principal, CreateInput{OrgID: 1, WorkspaceID: 10, Name: "new", Driver: "fake", Params: engine.Params{"host": "db"}})
	if !errors.Is(err, want) {
		t.Fatalf("Create error = %v, want %v", err, want)
	}
	if len(store.connections) != 1 {
		t.Fatalf("connections changed after policy denial: %d", len(store.connections))
	}
}

func TestUpdateRejectsClearingRequiredSSHSecret(t *testing.T) {
	service, store, provider, principal := catalogFixture()
	conn := store.connections[30]
	conn.Driver = "postgres"
	conn.Params = json.RawMessage(`{"host":"db","port":"5432","username":"user"}`)
	conn.SSHConfig = json.RawMessage(`{"enabled":true,"host":"bastion","user":"deploy","auth_method":"password","insecure_skip_host_key":true}`)
	store.connections[30] = conn
	service.deps.SpecLookup = engine.ConnectionSpecFor
	provider.states[credentials.SecretSSHPassword] = credentials.SecretState{Set: true, Source: credentials.SourceStored}
	provider.values[credentials.SecretSSHPassword] = "stored"

	err := service.Update(context.Background(), principal, ConnRef{OrgID: 1, WorkspaceID: 10, ConnectionID: 30}, UpdateInput{Secrets: map[credentials.SecretName]*string{credentials.SecretSSHPassword: nil}})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("Update error = %v, want ErrValidation", err)
	}
	if provider.clearCalls[credentials.SecretSSHPassword] != 0 {
		t.Fatal("required SSH password was cleared")
	}
}

func TestUpdateActiveSessionsRequireForceAndRevokeAfterChange(t *testing.T) {
	service, store, _, principal := catalogFixture()
	revoker := &fakeRevoker{active: 2}
	service.deps.Revoker = revoker
	ref := ConnRef{OrgID: 1, WorkspaceID: 10, ConnectionID: 30}
	nextParams := engine.Params{"host": "new"}
	if err := service.Update(context.Background(), principal, ref, UpdateInput{Params: nextParams}); !errors.Is(err, ErrActiveSessions) {
		t.Fatalf("Update error = %v, want ErrActiveSessions", err)
	}
	if revoker.revoked != 0 || store.cacheDeletes != 0 {
		t.Fatalf("denied update side effects: revoked=%d cache_deletes=%d", revoker.revoked, store.cacheDeletes)
	}
	if err := service.Update(context.Background(), principal, ref, UpdateInput{Params: nextParams, Force: true}); err != nil {
		t.Fatalf("forced Update: %v", err)
	}
	if revoker.revoked != 1 || store.cacheDeletes != 1 {
		t.Fatalf("forced update side effects: revoked=%d cache_deletes=%d", revoker.revoked, store.cacheDeletes)
	}
}

func TestTestConnectionIDAuthorizesBeforeLoadingSecrets(t *testing.T) {
	service, _, provider, principal := catalogFixture()
	service.deps.Policy = fakePolicy{permissions: map[string]bool{access.PermConnCreate: true}}
	connectionID := int64(30)
	_, err := service.Test(context.Background(), principal, TestInput{OrgID: 1, WorkspaceID: 10, ConnectionID: &connectionID, Driver: "fake", Params: engine.Params{"host": "db"}})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("Test error = %v, want ErrForbidden", err)
	}
	if provider.revealCalls != 0 {
		t.Fatalf("Reveal called %d times before authorization", provider.revealCalls)
	}
}

func TestTestConnectionIDLoadsMissingSecretsAfterAuthorization(t *testing.T) {
	service, _, provider, principal := catalogFixture()
	recorder := &auditRecorder{}
	service.deps.Audit = recorder
	provider.states[credentials.SecretPassword] = credentials.SecretState{Set: true, Source: credentials.SourceStored}
	provider.values[credentials.SecretPassword] = "stored-password"
	prober := &captureProber{}
	service.deps.Prober = prober
	connectionID := int64(30)
	result, err := service.Test(context.Background(), principal, TestInput{OrgID: 1, WorkspaceID: 10, ConnectionID: &connectionID, Driver: "fake", Params: engine.Params{"host": "old"}})
	if err != nil || !result.OK {
		t.Fatalf("Test = %+v, %v", result, err)
	}
	if provider.revealCalls != 1 {
		t.Fatalf("Reveal calls = %d, want 1", provider.revealCalls)
	}
	if len(recorder.events) != 0 {
		t.Fatalf("unchanged target audit events = %d, want 0", len(recorder.events))
	}
	if prober.creds.DSN != "fake://old/stored-password" {
		t.Fatalf("probe DSN did not include stored password")
	}
}

func connIDPtr(id int64) *int64 { return &id }

func TestTestConnectionIDChangedTargetReusesStoredSecretForRevealingUser(t *testing.T) {
	service, _, provider, principal := catalogFixture()
	recorder := &auditRecorder{}
	service.deps.Audit = recorder
	provider.states[credentials.SecretPassword] = credentials.SecretState{Set: true, Source: credentials.SourceStored}
	provider.values[credentials.SecretPassword] = "stored-password"
	prober := &captureProber{}
	service.deps.Prober = prober
	result, err := service.Test(context.Background(), principal, TestInput{OrgID: 1, WorkspaceID: 10, ConnectionID: connIDPtr(30), Driver: "fake", Params: engine.Params{"host": "corrected"}})
	if err != nil || !result.OK || !prober.probed || prober.creds.DSN != "fake://corrected/stored-password" {
		t.Fatalf("Test = %+v, %v dsn=%q", result, err, prober.creds.DSN)
	}
	if provider.revealCalls != 1 {
		t.Fatalf("Reveal calls = %d, want 1", provider.revealCalls)
	}
	if len(recorder.events) != 1 {
		t.Fatalf("audit events = %d, want 1", len(recorder.events))
	}
	event := recorder.events[0]
	if event.Action != actionConnectionSecretRevealed || event.Resource != "connection" || event.ResourceID != "30" || event.Outcome != "success" || event.Metadata["secret_name"] != "password" || event.Metadata["workspace_id"] != "10" || event.Metadata["via"] != "test" || event.Actor.SubjectID == nil || *event.Actor.SubjectID != principal.Subject.ID {
		t.Fatalf("audit event = %+v", event)
	}
}

func TestTestConnectionIDChangedTargetAuditFailureStopsProbe(t *testing.T) {
	service, _, provider, principal := catalogFixture()
	service.deps.Audit = failingAudit{}
	provider.states[credentials.SecretPassword] = credentials.SecretState{Set: true, Source: credentials.SourceStored}
	provider.values[credentials.SecretPassword] = "stored-password"
	prober := &captureProber{}
	service.deps.Prober = prober

	_, err := service.Test(context.Background(), principal, TestInput{OrgID: 1, WorkspaceID: 10, ConnectionID: connIDPtr(30), Driver: "fake", Params: engine.Params{"host": "corrected"}})
	if err == nil || provider.revealCalls != 1 || prober.probed {
		t.Fatalf("Test error = %v reveals=%d probed=%v", err, provider.revealCalls, prober.probed)
	}
}

func TestTestConnectionIDRejectsReferenceSourcedSecret(t *testing.T) {
	service, _, provider, principal := catalogFixture()
	provider.states[credentials.SecretPassword] = credentials.SecretState{Set: true, Source: credentials.SourceReference}
	prober := &captureProber{}
	service.deps.Prober = prober
	_, err := service.Test(context.Background(), principal, TestInput{OrgID: 1, WorkspaceID: 10, ConnectionID: connIDPtr(30), Driver: "fake", Params: engine.Params{"host": "old"}})
	if !errors.Is(err, ErrValidation) || provider.revealCalls != 0 || prober.probed {
		t.Fatalf("Test error = %v reveals=%d probed=%v", err, provider.revealCalls, prober.probed)
	}
}

func TestTestConnectionIDSuppliedSecretWorksForChangedTarget(t *testing.T) {
	service, _, provider, principal := catalogFixture()
	provider.states[credentials.SecretPassword] = credentials.SecretState{Set: true, Source: credentials.SourceStored}
	provider.values[credentials.SecretPassword] = "stored-password"
	prober := &captureProber{}
	service.deps.Prober = prober
	supplied := "typed"
	result, err := service.Test(context.Background(), principal, TestInput{OrgID: 1, WorkspaceID: 10, ConnectionID: connIDPtr(30), Driver: "fake", Params: engine.Params{"host": "other"}, Secrets: map[credentials.SecretName]*string{credentials.SecretPassword: &supplied}})
	if err != nil || !result.OK || prober.creds.DSN != "fake://other/typed" || provider.revealCalls != 0 {
		t.Fatalf("Test = %+v, %v dsn=%q reveals=%d", result, err, prober.creds.DSN, provider.revealCalls)
	}
}

func TestTestHonorsTargetPolicyDenial(t *testing.T) {
	service, _, _, principal := catalogFixture()
	want := errors.New("denied")
	service.deps.TargetPolicy = &fakeTargetPolicy{err: want}
	prober := &captureProber{}
	service.deps.Prober = prober
	_, err := service.Test(context.Background(), principal, TestInput{OrgID: 1, WorkspaceID: 10, Driver: "fake", Params: engine.Params{"host": "db"}})
	if !errors.Is(err, want) || prober.probed {
		t.Fatalf("Test error = %v probed=%v", err, prober.probed)
	}
}

func TestUpdateHonorsTargetPolicyDenial(t *testing.T) {
	service, store, _, principal := catalogFixture()
	want := errors.New("denied")
	service.deps.TargetPolicy = &fakeTargetPolicy{err: want}
	err := service.Update(context.Background(), principal, ConnRef{OrgID: 1, WorkspaceID: 10, ConnectionID: 30}, UpdateInput{Params: engine.Params{"host": "new"}})
	if !errors.Is(err, want) || string(store.connections[30].Params) != `{"host":"old"}` {
		t.Fatalf("Update error = %v params=%s", err, store.connections[30].Params)
	}
}

func TestCreateSuccessAndDelete(t *testing.T) {
	service, store, provider, principal := catalogFixture()
	ancestry := &fakeAncestry{}
	service.deps.Ancestry = ancestry
	secret := "pw"
	env := int64(20)
	view, err := service.Create(context.Background(), principal, CreateInput{OrgID: 1, WorkspaceID: 10, EnvironmentID: &env, Name: " new ", Driver: "fake", Params: engine.Params{"host": "db"}, Secrets: map[credentials.SecretName]*string{credentials.SecretPassword: &secret}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if view.Name != "new" || view.Params["host"] != "db" || !view.Secrets[credentials.SecretPassword].Set || provider.values[credentials.SecretPassword] != "pw" {
		t.Fatalf("view = %+v", view)
	}
	if err := service.Delete(context.Background(), principal, ConnRef{OrgID: 1, WorkspaceID: 10, ConnectionID: view.ID}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := store.connections[view.ID]; ok || len(ancestry.invalidated) != 1 || ancestry.invalidated[0] != view.ID {
		t.Fatalf("connection remains or ancestry not invalidated: %v", ancestry.invalidated)
	}
}

func TestMutationsDenyMissingPermissionAndNonMembers(t *testing.T) {
	create := CreateInput{OrgID: 1, WorkspaceID: 10, Name: "n", Driver: "fake", Params: engine.Params{"host": "db"}}
	ref := ConnRef{OrgID: 1, WorkspaceID: 10, ConnectionID: 30}
	name := "x"
	calls := map[string]func(*Service, access.Principal) error{
		"create": func(s *Service, p access.Principal) error {
			_, err := s.Create(context.Background(), p, create)
			return err
		},
		"update": func(s *Service, p access.Principal) error {
			return s.Update(context.Background(), p, ref, UpdateInput{Name: &name})
		},
		"delete": func(s *Service, p access.Principal) error { return s.Delete(context.Background(), p, ref) },
	}
	for label, call := range calls {
		t.Run(label+"/no permission", func(t *testing.T) {
			service, store, _, principal := catalogFixture()
			service.deps.Policy = fakePolicy{permissions: map[string]bool{}}
			if err := call(service, principal); !errors.Is(err, ErrForbidden) {
				t.Fatalf("error = %v, want ErrForbidden", err)
			}
			if len(store.connections) != 1 || store.connections[30].Name != "primary" {
				t.Fatal("denied call changed state")
			}
		})
		t.Run(label+"/non member", func(t *testing.T) {
			service, _, _, principal := catalogFixture()
			principal.Subject.ID = 99
			if err := call(service, principal); !errors.Is(err, ErrForbidden) {
				t.Fatalf("error = %v, want ErrForbidden", err)
			}
		})
		t.Run(label+"/non account subject", func(t *testing.T) {
			service, _, _, principal := catalogFixture()
			principal.Subject.Kind = access.SubjectKind("service")
			if err := call(service, principal); !errors.Is(err, ErrForbidden) {
				t.Fatalf("error = %v, want ErrForbidden", err)
			}
		})
	}
}

func TestConnectionInOtherWorkspaceOrEnvironmentIsNotFound(t *testing.T) {
	service, store, _, principal := catalogFixture()
	orgID := int64(1)
	store.workspaces[11] = database.Workspace{ID: 11, OrgID: &orgID, OwnerType: "org", OwnerID: orgID}
	wrongEnv := int64(21)
	name := "x"
	for _, ref := range []ConnRef{
		{OrgID: 1, WorkspaceID: 11, ConnectionID: 30},
		{OrgID: 1, WorkspaceID: 10, EnvironmentID: &wrongEnv, ConnectionID: 30},
	} {
		if _, err := service.Get(context.Background(), principal, ref); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Get(%+v) = %v", ref, err)
		}
		if err := service.Update(context.Background(), principal, ref, UpdateInput{Name: &name}); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Update(%+v) = %v", ref, err)
		}
		if err := service.Delete(context.Background(), principal, ref); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Delete(%+v) = %v", ref, err)
		}
	}
	if len(store.connections) != 1 {
		t.Fatal("foreign delete removed the connection")
	}
}

func TestConnectionViewJSONOmitsSecretValuesAndReflectsRevealPermission(t *testing.T) {
	service, _, provider, principal := catalogFixture()
	provider.states[credentials.SecretPassword] = credentials.SecretState{Set: true, Source: credentials.SourceStored}
	provider.values[credentials.SecretPassword] = "highly-secret-value"
	ref := ConnRef{OrgID: 1, WorkspaceID: 10, ConnectionID: 30}
	view, err := service.Get(context.Background(), principal, ref)
	if err != nil || !view.Secrets[credentials.SecretPassword].Revealable {
		t.Fatalf("Get = %+v, %v", view, err)
	}
	raw, _ := json.Marshal(view)
	if strings.Contains(string(raw), "highly-secret-value") {
		t.Fatalf("view JSON contains secret value: %s", raw)
	}

	service.deps.Policy = fakePolicy{permissions: map[string]bool{access.PermConnRead: true}}
	view, err = service.Get(context.Background(), principal, ref)
	if err != nil || !view.Secrets[credentials.SecretPassword].Set || view.Secrets[credentials.SecretPassword].Revealable {
		t.Fatalf("read-only Get = %+v, %v", view, err)
	}
}

func TestConnectionViewResolvesRevealPolicyOncePerView(t *testing.T) {
	service, _, provider, principal := catalogFixture()
	for _, name := range allSecretNames {
		provider.states[name] = credentials.SecretState{Set: true, Source: credentials.SourceStored}
	}
	policy := &countingRevealPolicy{}
	service.deps.RevealPolicy = policy
	if _, err := service.Get(context.Background(), principal, ConnRef{OrgID: 1, WorkspaceID: 10, ConnectionID: 30}); err != nil {
		t.Fatal(err)
	}
	if policy.calls != 1 {
		t.Fatalf("reveal policy evaluated %d times, want 1", policy.calls)
	}
}

func TestCreateLeavesNoConnectionWhenLaterStepFails(t *testing.T) {
	secret := "pw"
	secrets := map[credentials.SecretName]*string{credentials.SecretPassword: &secret}
	for name, mutate := range map[string]func(*fakeStore, *fakeCredentials){
		"structured columns": func(s *fakeStore, _ *fakeCredentials) { s.structuredErr = errors.New("boom") },
		"secrets": func(_ *fakeStore, c *fakeCredentials) {
			c.setErr = map[credentials.SecretName]error{credentials.SecretPassword: errors.New("boom")}
		},
	} {
		t.Run(name, func(t *testing.T) {
			service, store, provider, principal := catalogFixture()
			service.deps.Ancestry = &fakeAncestry{}
			mutate(store, provider)
			_, err := service.Create(context.Background(), principal, CreateInput{OrgID: 1, WorkspaceID: 10, Name: "n", Driver: "fake", Params: engine.Params{"host": "db"}, Secrets: secrets})
			if err == nil {
				t.Fatal("Create succeeded")
			}
			if len(store.connections) != 1 {
				t.Fatalf("half-configured connection remains: %d rows", len(store.connections))
			}
		})
	}
}

func TestUpdateRestoresStateWhenLaterStepFails(t *testing.T) {
	newPassword, newSSH := "new-password", "ssh-secret"
	name := "renamed"
	input := UpdateInput{Name: &name, Params: engine.Params{"host": "new"}, Force: true, Secrets: map[credentials.SecretName]*string{
		credentials.SecretPassword: &newPassword, credentials.SecretSSHPassword: &newSSH,
	}}
	for label, mutate := range map[string]func(*fakeStore, *fakeCredentials){
		"structured columns": func(s *fakeStore, _ *fakeCredentials) { s.structuredErr = errors.New("boom") },
		"second secret": func(_ *fakeStore, c *fakeCredentials) {
			c.setErr = map[credentials.SecretName]error{credentials.SecretSSHPassword: errors.New("boom")}
		},
	} {
		t.Run(label, func(t *testing.T) {
			service, store, provider, principal := catalogFixture()
			provider.states[credentials.SecretPassword] = credentials.SecretState{Set: true, Source: credentials.SourceStored}
			provider.values[credentials.SecretPassword] = "old-password"
			mutate(store, provider)
			err := service.Update(context.Background(), principal, ConnRef{OrgID: 1, WorkspaceID: 10, ConnectionID: 30}, input)
			if err == nil {
				t.Fatal("Update succeeded")
			}
			store.structuredErr = nil
			conn := store.connections[30]
			if conn.Name != "primary" || string(conn.Params) != `{"host":"old"}` {
				t.Fatalf("columns not restored: name=%q params=%s", conn.Name, conn.Params)
			}
			if provider.values[credentials.SecretPassword] != "old-password" || provider.states[credentials.SecretSSHPassword].Set {
				t.Fatalf("secrets not restored: %v %v", provider.values, provider.states)
			}
		})
	}
}

type countingRevealPolicy struct{ calls int }

func (p *countingRevealPolicy) Allowed(context.Context, credentials.OrgRef, access.Principal) (bool, error) {
	p.calls++
	return true, nil
}

func TestUpdateRejectsReplaceAndClearOfReferenceSecret(t *testing.T) {
	replacement := "new"
	for label, change := range map[string]*string{"replace": &replacement, "clear": nil} {
		t.Run(label, func(t *testing.T) {
			service, store, provider, principal := catalogFixture()
			provider.states[credentials.SecretPassword] = credentials.SecretState{Set: true, Source: credentials.SourceReference}
			name := "renamed"
			err := service.Update(context.Background(), principal, ConnRef{OrgID: 1, WorkspaceID: 10, ConnectionID: 30}, UpdateInput{Name: &name, Secrets: map[credentials.SecretName]*string{credentials.SecretPassword: change}})
			var validation *ValidationError
			if !errors.As(err, &validation) || validation.Fields["secrets.password"] == "" {
				t.Fatalf("Update error = %v, want field error on secrets.password", err)
			}
			if store.connections[30].Name != "primary" || len(provider.setCalls[credentials.SecretPassword]) != 0 || provider.clearCalls[credentials.SecretPassword] != 0 {
				t.Fatal("rejected update wrote state")
			}
			if provider.states[credentials.SecretPassword].Source != credentials.SourceReference {
				t.Fatal("reference binding lost")
			}
		})
	}
}

func TestUpdateKeepsReferenceSecretWhenAbsent(t *testing.T) {
	service, store, provider, principal := catalogFixture()
	provider.states[credentials.SecretPassword] = credentials.SecretState{Set: true, Source: credentials.SourceReference}
	name := "renamed"
	if err := service.Update(context.Background(), principal, ConnRef{OrgID: 1, WorkspaceID: 10, ConnectionID: 30}, UpdateInput{Name: &name}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if store.connections[30].Name != "renamed" || provider.states[credentials.SecretPassword].Source != credentials.SourceReference {
		t.Fatal("keep did not preserve reference secret")
	}
}

func TestGetRejectsNonAccountSubject(t *testing.T) {
	service, _, _, principal := catalogFixture()
	principal.Subject.Kind = access.SubjectKind("service")
	if _, err := service.Get(context.Background(), principal, ConnRef{OrgID: 1, WorkspaceID: 10, ConnectionID: 30}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("Get error = %v, want ErrForbidden", err)
	}
}

func TestTestConnectionIDSSHHostOrUserChangeCountsAsChangedTarget(t *testing.T) {
	base := SSHConfig{Enabled: true, Host: "bastion", Port: 22, User: "deploy", AuthMethod: "password", InsecureSkipHostKey: true}
	for label, mutate := range map[string]func(*SSHConfig){
		"host": func(c *SSHConfig) { c.Host = "evil" },
		"user": func(c *SSHConfig) { c.User = "root" },
	} {
		t.Run(label, func(t *testing.T) {
			service, store, provider, principal := catalogFixture()
			service.deps.Policy = fakePolicy{permissions: map[string]bool{access.PermConnCreate: true, access.PermConnRead: true}}
			conn := store.connections[30]
			conn.Driver = "postgres"
			conn.Params = json.RawMessage(`{"host":"db","port":"5432","username":"user"}`)
			stored, _ := json.Marshal(base)
			conn.SSHConfig = stored
			store.connections[30] = conn
			service.deps.SpecLookup = engine.ConnectionSpecFor
			provider.states[credentials.SecretSSHPassword] = credentials.SecretState{Set: true, Source: credentials.SourceStored}
			provider.values[credentials.SecretSSHPassword] = "stored-ssh"
			service.deps.Prober = &captureProber{}
			changed := base
			mutate(&changed)
			_, err := service.Test(context.Background(), principal, TestInput{OrgID: 1, WorkspaceID: 10, ConnectionID: connIDPtr(30), Driver: "postgres", Params: engine.Params{"host": "db", "port": "5432", "username": "user"}, SSHConfig: &changed})
			if !errors.Is(err, ErrValidation) || provider.revealCalls != 0 {
				t.Fatalf("Test error = %v reveals=%d", err, provider.revealCalls)
			}
		})
	}
}

func TestTestConnectionIDRepointedTunnelRejectsDatabasePasswordForNonRevealingUser(t *testing.T) {
	service, store, provider, principal := catalogFixture()
	recorder := &auditRecorder{}
	service.deps.Audit = recorder
	service.deps.Policy = fakePolicy{permissions: map[string]bool{access.PermConnCreate: true, access.PermConnRead: true}}
	conn := store.connections[30]
	conn.Driver = "postgres"
	conn.Params = json.RawMessage(`{"host":"db","port":"5432","username":"user"}`)
	conn.SSHConfig = json.RawMessage(`{"enabled":true,"host":"bastion-a","port":22,"user":"deploy","auth_method":"password","insecure_skip_host_key":true}`)
	store.connections[30] = conn
	service.deps.SpecLookup = engine.ConnectionSpecFor
	for name, value := range map[credentials.SecretName]string{credentials.SecretPassword: "db-password", credentials.SecretSSHPassword: "ssh-password"} {
		provider.states[name] = credentials.SecretState{Set: true, Source: credentials.SourceStored}
		provider.values[name] = value
	}
	prober := &captureProber{}
	service.deps.Prober = prober
	next := &SSHConfig{Enabled: true, Host: "bastion-b", Port: 22, User: "deploy", AuthMethod: "password", InsecureSkipHostKey: true}
	_, err := service.Test(context.Background(), principal, TestInput{
		OrgID: 1, WorkspaceID: 10, ConnectionID: connIDPtr(30), Driver: "postgres",
		Params: engine.Params{"host": "db", "port": "5432", "username": "user"}, SSHConfig: next,
	})
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Fields["secrets.password"] == "" {
		t.Fatalf("Test error = %v, want database password field error", err)
	}
	if provider.revealCalls != 0 || prober.probed || len(recorder.events) != 0 {
		t.Fatalf("denied test revealed, audited, or probed: reveals=%d audits=%d probed=%v", provider.revealCalls, len(recorder.events), prober.probed)
	}
}

type failingProber struct{ err error }

func (p failingProber) Probe(context.Context, execution.Scope, credentials.Credentials, execution.Limits, func(metadata.SchemaInspector) error) error {
	return p.err
}

type callbackProber struct{}

type emptyInspector struct{ metadata.SchemaInspector }

func (emptyInspector) Tree() metadata.Tree { return metadata.Tree{} }

func (callbackProber) Probe(_ context.Context, _ execution.Scope, _ credentials.Credentials, _ execution.Limits, fn func(metadata.SchemaInspector) error) error {
	return fn(emptyInspector{})
}

func TestRedactSecrets(t *testing.T) {
	const pem = "-----BEGIN KEY-----\nAAAA\nBBBB\n-----END KEY-----\n"
	tests := []struct {
		name    string
		text    string
		dsn     string
		secrets map[string]string
		want    string
	}{
		{name: "empty secret does not blank text", text: "dial failed", secrets: map[string]string{"password": ""}, want: "dial failed"},
		{name: "short secret", text: "auth failed for pw", secrets: map[string]string{"password": "pw"}, want: "auth failed for [redacted]"},
		{name: "url-escaped password in dsn fragment", text: "parse postgres://u:p%40ss%2Fw%20d@h/db failed", secrets: map[string]string{"password": "p@ss/w d"}, want: "parse postgres://u:[redacted]@h/db failed"},
		{name: "query-escaped password", text: "bad dsn password=p%40ss+w", secrets: map[string]string{"password": "p@ss w"}, want: "bad dsn password=[redacted]"},
		{name: "json-escaped pem", text: `key rejected: "-----BEGIN KEY-----\nAAAA\nBBBB\n-----END KEY-----\n"`, secrets: map[string]string{"ssh_private_key": pem}, want: `key rejected: "[redacted]"`},
		{name: "newline-collapsed pem", text: "key rejected: -----BEGIN KEY----- AAAA BBBB -----END KEY-----", secrets: map[string]string{"ssh_private_key": pem}, want: "key rejected: [redacted]"},
		{name: "dsn echo", text: "dial tcp: postgres://u:x@h:1/db refused", dsn: "postgres://u:x@h:1/db", want: "dial tcp: [redacted] refused"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := redactSecrets(tc.text, tc.dsn, tc.secrets); got != tc.want {
				t.Fatalf("redactSecrets = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTestRedactsStoredSecretEchoedByConnectError(t *testing.T) {
	service, _, provider, principal := catalogFixture()
	provider.states[credentials.SecretPassword] = credentials.SecretState{Set: true, Source: credentials.SourceStored}
	provider.values[credentials.SecretPassword] = "stored-password"
	service.deps.Prober = failingProber{err: &execution.ConnectError{Err: errors.New("auth failed using stored-password")}}
	result, err := service.Test(context.Background(), principal, TestInput{OrgID: 1, WorkspaceID: 10, ConnectionID: connIDPtr(30), Driver: "fake", Params: engine.Params{"host": "old"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.OK || strings.Contains(result.Error, "stored-password") || result.Stage != TestStageConnect || result.ErrorCategory != "target_unreachable" {
		t.Fatalf("result = %+v", result)
	}
}

func TestTestRedactsStoredSecretEchoedByScopeDiscoveryError(t *testing.T) {
	service, _, provider, principal := catalogFixture()
	provider.states[credentials.SecretPassword] = credentials.SecretState{Set: true, Source: credentials.SourceStored}
	provider.values[credentials.SecretPassword] = "stored-password"
	service.deps.Prober = callbackProber{}
	result, err := service.Test(context.Background(), principal, TestInput{
		OrgID: 1, WorkspaceID: 10, ConnectionID: connIDPtr(30), Driver: "fake",
		Params: engine.Params{"host": "old"}, ParentScope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "unknown", Name: "stored-password"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || strings.Contains(result.ScopeDiscoveryError, "stored-password") || !strings.Contains(result.ScopeDiscoveryError, "[redacted]") {
		t.Fatalf("result = %+v", result)
	}
}

func TestRedactConnectionError(t *testing.T) {
	creds := credentials.Credentials{
		Driver: "postgres",
		DSN:    "postgresql://user:db-password@db.example.com:5432/app",
		TLS:    &engine.TLSConfig{ClientKeyPEM: "tls-private-key"},
		SSH:    &credentials.SSHConfig{Password: "ssh-password", PrivateKeyPEM: "ssh-private-key", Passphrase: "ssh-passphrase"},
	}
	message := "connect postgresql://user:db-password@db.example.com:5432/app db-password tls-private-key ssh-password ssh-private-key ssh-passphrase"
	redacted := RedactConnectionError(message, creds)
	for _, secret := range []string{creds.DSN, "db-password", "tls-private-key", "ssh-password", "ssh-private-key", "ssh-passphrase"} {
		if strings.Contains(redacted, secret) {
			t.Fatalf("redacted error contains %q: %s", secret, redacted)
		}
	}
}

func TestConnectionTargetChanged(t *testing.T) {
	spec := fakeSpec{}
	tests := []struct {
		name string
		old  connectionTarget
		new  connectionTarget
		want bool
	}{
		{name: "verification downgrade", old: canonicalConnectionTarget(engine.Params{"host": "db"}, &TLSConfig{Mode: "verify-full"}, nil), new: canonicalConnectionTarget(engine.Params{"host": "db"}, &TLSConfig{Mode: "disable"}, nil), want: true},
		{name: "ca", old: canonicalConnectionTarget(engine.Params{"host": "db"}, &TLSConfig{Mode: "verify-ca", CAPEM: "ca-a"}, nil), new: canonicalConnectionTarget(engine.Params{"host": "db"}, &TLSConfig{Mode: "verify-ca", CAPEM: "ca-b"}, nil), want: true},
		{name: "server name", old: canonicalConnectionTarget(engine.Params{"host": "db"}, &TLSConfig{Mode: "verify-full", ServerName: "db-a"}, nil), new: canonicalConnectionTarget(engine.Params{"host": "db"}, &TLSConfig{Mode: "verify-full", ServerName: "db-b"}, nil), want: true},
		{name: "host", old: canonicalConnectionTarget(engine.Params{"host": "db-a"}, nil, nil), new: canonicalConnectionTarget(engine.Params{"host": "db-b"}, nil, nil), want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := connectionTargetChanged(spec, tc.old, tc.new); got != tc.want {
				t.Fatalf("connectionTargetChanged = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTestErrorCategory(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{context.DeadlineExceeded, "timeout"},
		{context.Canceled, "cancelled"},
		{fmt.Errorf("wrapped: %w", settings.ErrTargetDenied), "policy_denied"},
		{errors.New("engine: unknown driver \"x\""), "unsupported_driver"},
		{errors.New("connection refused"), "target_unreachable"},
	}
	for _, tc := range tests {
		if got := testErrorCategory(tc.err); got != tc.want {
			t.Errorf("testErrorCategory(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
}

func TestTestResultAlwaysSerializesLatency(t *testing.T) {
	data, err := json.Marshal(TestResult{OK: false})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"latency_ms":0`) {
		t.Fatalf("json = %s", data)
	}
}

func TestUpdateTargetChangeRequiresStoredSecretsResupplied(t *testing.T) {
	ref := ConnRef{OrgID: 1, WorkspaceID: 10, ConnectionID: 30}
	newPassword := "new"

	t.Run("repoint host keeping password is rejected", func(t *testing.T) {
		service, store, provider, principal := catalogFixture()
		service.deps.Policy = fakePolicy{permissions: map[string]bool{access.PermConnUpdate: true}}
		provider.states[credentials.SecretPassword] = credentials.SecretState{Set: true, Source: credentials.SourceStored}
		err := service.Update(context.Background(), principal, ref, UpdateInput{Params: engine.Params{"host": "evil"}})
		var validation *ValidationError
		if !errors.As(err, &validation) || validation.Fields["secrets.password"] == "" {
			t.Fatalf("Update error = %v, want field error on secrets.password", err)
		}
		if string(store.connections[30].Params) != `{"host":"old"}` {
			t.Fatalf("rejected update wrote params: %s", store.connections[30].Params)
		}
	})
	t.Run("repoint host with new password", func(t *testing.T) {
		service, _, provider, principal := catalogFixture()
		provider.states[credentials.SecretPassword] = credentials.SecretState{Set: true, Source: credentials.SourceStored}
		err := service.Update(context.Background(), principal, ref, UpdateInput{Params: engine.Params{"host": "new"}, Secrets: map[credentials.SecretName]*string{credentials.SecretPassword: &newPassword}})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
	})
	t.Run("repoint host clearing password", func(t *testing.T) {
		service, _, provider, principal := catalogFixture()
		provider.states[credentials.SecretPassword] = credentials.SecretState{Set: true, Source: credentials.SourceStored}
		err := service.Update(context.Background(), principal, ref, UpdateInput{Params: engine.Params{"host": "new"}, Secrets: map[credentials.SecretName]*string{credentials.SecretPassword: nil}})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
	})
	t.Run("name-only edit keeps secrets", func(t *testing.T) {
		service, _, provider, principal := catalogFixture()
		provider.states[credentials.SecretPassword] = credentials.SecretState{Set: true, Source: credentials.SourceStored}
		name := "renamed"
		if err := service.Update(context.Background(), principal, ref, UpdateInput{Name: &name}); err != nil {
			t.Fatalf("Update: %v", err)
		}
	})
	t.Run("database-only edit keeps secrets", func(t *testing.T) {
		service, store, provider, principal := catalogFixture()
		conn := store.connections[30]
		conn.Driver = "postgres"
		conn.Params = json.RawMessage(`{"host":"db","port":"5432","username":"user","database":"a"}`)
		store.connections[30] = conn
		service.deps.SpecLookup = engine.ConnectionSpecFor
		provider.states[credentials.SecretPassword] = credentials.SecretState{Set: true, Source: credentials.SourceStored}
		err := service.Update(context.Background(), principal, ref, UpdateInput{Params: engine.Params{"host": "db", "port": "5432", "username": "user", "database": "b"}})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
	})
	t.Run("oracle service-only edit keeps secrets", func(t *testing.T) {
		service, store, provider, principal := catalogFixture()
		conn := store.connections[30]
		conn.Driver = "oracle"
		conn.Params = json.RawMessage(`{"host":"db","port":"1521","username":"user","serviceName":"a"}`)
		store.connections[30] = conn
		service.deps.SpecLookup = engine.ConnectionSpecFor
		provider.states[credentials.SecretPassword] = credentials.SecretState{Set: true, Source: credentials.SourceStored}
		err := service.Update(context.Background(), principal, ref, UpdateInput{Params: engine.Params{"host": "db", "port": "1521", "username": "user", "serviceName": "b"}})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
	})
	t.Run("ssh host change rejects kept secret and revokes sessions once resupplied", func(t *testing.T) {
		service, store, provider, principal := catalogFixture()
		service.deps.Policy = fakePolicy{permissions: map[string]bool{access.PermConnUpdate: true}}
		revoker := &fakeRevoker{active: 1}
		service.deps.Revoker = revoker
		conn := store.connections[30]
		stored, _ := json.Marshal(SSHConfig{Enabled: true, Host: "bastion", User: "deploy", AuthMethod: "password", InsecureSkipHostKey: true})
		conn.SSHConfig = stored
		conn.Driver = "postgres"
		conn.Params = json.RawMessage(`{"host":"db","port":"5432","username":"user"}`)
		store.connections[30] = conn
		service.deps.SpecLookup = engine.ConnectionSpecFor
		provider.states[credentials.SecretPassword] = credentials.SecretState{Set: true, Source: credentials.SourceStored}
		next := &SSHConfig{Enabled: true, Host: "evil", User: "deploy", AuthMethod: "password", InsecureSkipHostKey: true}
		sshPassword := "ssh"
		err := service.Update(context.Background(), principal, ref, UpdateInput{SSHConfig: next, Force: true, Secrets: map[credentials.SecretName]*string{credentials.SecretSSHPassword: &sshPassword}})
		if !errors.Is(err, ErrValidation) {
			t.Fatalf("Update error = %v, want ErrValidation for kept password", err)
		}
		err = service.Update(context.Background(), principal, ref, UpdateInput{SSHConfig: next, Force: true, Secrets: map[credentials.SecretName]*string{credentials.SecretSSHPassword: &sshPassword, credentials.SecretPassword: nil}})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if revoker.revoked != 1 || store.cacheDeletes != 1 {
			t.Fatalf("ssh change side effects: revoked=%d cache_deletes=%d", revoker.revoked, store.cacheDeletes)
		}
	})
}

func TestUpdateSecretReentryIsScopedBySecretTarget(t *testing.T) {
	ref := ConnRef{OrgID: 1, WorkspaceID: 10, ConnectionID: 30}
	postgres := func(store *fakeStore, tls *TLSConfig, ssh *SSHConfig) {
		conn := store.connections[30]
		conn.Driver = "postgres"
		conn.Params = json.RawMessage(`{"host":"db","port":"5432","username":"user"}`)
		if tls != nil {
			conn.TLSConfig, _ = json.Marshal(tls)
		}
		if ssh != nil {
			conn.SSHConfig, _ = json.Marshal(ssh)
		}
		store.connections[30] = conn
	}
	stored := credentials.SecretState{Set: true, Source: credentials.SourceStored}

	t.Run("tls downgrade keeps database password", func(t *testing.T) {
		service, store, provider, principal := catalogFixture()
		postgres(store, &TLSConfig{Mode: "verify-full"}, nil)
		service.deps.SpecLookup = engine.ConnectionSpecFor
		provider.states[credentials.SecretPassword] = stored
		if err := service.Update(context.Background(), principal, ref, UpdateInput{TLSConfig: &TLSConfig{Mode: "disable"}}); err != nil {
			t.Fatalf("Update: %v", err)
		}
		var saved TLSConfig
		if err := json.Unmarshal(store.connections[30].TLSConfig, &saved); err != nil || saved.Mode != "disable" {
			t.Fatalf("saved TLS config = %+v, %v", saved, err)
		}
		if !provider.states[credentials.SecretPassword].Set {
			t.Fatal("database password state was cleared")
		}
	})

	t.Run("ca change keeps database password", func(t *testing.T) {
		service, store, provider, principal := catalogFixture()
		postgres(store, &TLSConfig{Mode: "verify-ca", CAPEM: "old-ca"}, nil)
		service.deps.SpecLookup = engine.ConnectionSpecFor
		provider.states[credentials.SecretPassword] = stored
		if err := service.Update(context.Background(), principal, ref, UpdateInput{TLSConfig: &TLSConfig{Mode: "verify-ca", CAPEM: "new-ca"}}); err != nil {
			t.Fatalf("Update: %v", err)
		}
		var saved TLSConfig
		if err := json.Unmarshal(store.connections[30].TLSConfig, &saved); err != nil || saved.Mode != "verify-ca" || saved.CAPEM != "new-ca" {
			t.Fatalf("saved TLS config = %+v, %v", saved, err)
		}
		if !provider.states[credentials.SecretPassword].Set {
			t.Fatal("database password state was cleared")
		}
	})

	t.Run("tls server name change requires client key", func(t *testing.T) {
		service, store, provider, principal := catalogFixture()
		service.deps.Policy = fakePolicy{permissions: map[string]bool{access.PermConnUpdate: true}}
		postgres(store, &TLSConfig{Mode: "verify-full", ServerName: "db-a", ClientCertPEM: "cert"}, nil)
		service.deps.SpecLookup = engine.ConnectionSpecFor
		provider.states[credentials.SecretTLSClientKey] = stored
		provider.values[credentials.SecretTLSClientKey] = "key"
		err := service.Update(context.Background(), principal, ref, UpdateInput{TLSConfig: &TLSConfig{Mode: "verify-full", ServerName: "db-b", ClientCertPEM: "cert"}})
		var validation *ValidationError
		if !errors.As(err, &validation) || validation.Fields["secrets.tls_client_key"] == "" {
			t.Fatalf("Update error = %v, want tls client key field error", err)
		}
	})

	t.Run("ssh host change requires database password", func(t *testing.T) {
		service, store, provider, principal := catalogFixture()
		service.deps.Policy = fakePolicy{permissions: map[string]bool{access.PermConnUpdate: true}}
		ssh := &SSHConfig{Enabled: true, Host: "bastion-a", Port: 22, User: "deploy", AuthMethod: "password", InsecureSkipHostKey: true}
		postgres(store, nil, ssh)
		service.deps.SpecLookup = engine.ConnectionSpecFor
		provider.states[credentials.SecretPassword] = stored
		provider.states[credentials.SecretSSHPassword] = stored
		provider.values[credentials.SecretSSHPassword] = "old-ssh"
		newSSH := "new-ssh"
		next := *ssh
		next.Host = "bastion-b"
		err := service.Update(context.Background(), principal, ref, UpdateInput{SSHConfig: &next, Secrets: map[credentials.SecretName]*string{credentials.SecretSSHPassword: &newSSH}})
		var validation *ValidationError
		if !errors.As(err, &validation) || validation.Fields["secrets.password"] == "" {
			t.Fatalf("Update error = %v, want database password field error", err)
		}
	})

	t.Run("database host change requires password", func(t *testing.T) {
		service, store, provider, principal := catalogFixture()
		service.deps.Policy = fakePolicy{permissions: map[string]bool{access.PermConnUpdate: true}}
		postgres(store, nil, nil)
		service.deps.SpecLookup = engine.ConnectionSpecFor
		provider.states[credentials.SecretPassword] = stored
		err := service.Update(context.Background(), principal, ref, UpdateInput{Params: engine.Params{"host": "other", "port": "5432", "username": "user"}})
		var validation *ValidationError
		if !errors.As(err, &validation) || validation.Fields["secrets.password"] == "" {
			t.Fatalf("Update error = %v, want database password field error", err)
		}
	})

	t.Run("ssh host change requires ssh secret", func(t *testing.T) {
		service, store, provider, principal := catalogFixture()
		service.deps.Policy = fakePolicy{permissions: map[string]bool{access.PermConnUpdate: true}}
		ssh := &SSHConfig{Enabled: true, Host: "bastion-a", Port: 22, User: "deploy", AuthMethod: "password", InsecureSkipHostKey: true}
		postgres(store, nil, ssh)
		service.deps.SpecLookup = engine.ConnectionSpecFor
		provider.states[credentials.SecretSSHPassword] = stored
		provider.values[credentials.SecretSSHPassword] = "old-ssh"
		next := *ssh
		next.Host = "bastion-b"
		err := service.Update(context.Background(), principal, ref, UpdateInput{SSHConfig: &next})
		var validation *ValidationError
		if !errors.As(err, &validation) || validation.Fields["secrets.ssh_password"] == "" {
			t.Fatalf("Update error = %v, want ssh password field error", err)
		}
	})

	t.Run("clear ssh secret while disabling tunnel", func(t *testing.T) {
		service, store, provider, principal := catalogFixture()
		service.deps.Policy = fakePolicy{permissions: map[string]bool{access.PermConnUpdate: true}}
		ssh := &SSHConfig{Enabled: true, Host: "bastion", Port: 22, User: "deploy", AuthMethod: "password", InsecureSkipHostKey: true}
		postgres(store, nil, ssh)
		service.deps.SpecLookup = engine.ConnectionSpecFor
		provider.states[credentials.SecretSSHPassword] = stored
		provider.values[credentials.SecretSSHPassword] = "old-ssh"
		next := *ssh
		next.Enabled = false
		if err := service.Update(context.Background(), principal, ref, UpdateInput{SSHConfig: &next, Secrets: map[credentials.SecretName]*string{credentials.SecretSSHPassword: nil}}); err != nil {
			t.Fatalf("Update: %v", err)
		}
		if provider.states[credentials.SecretSSHPassword].Set || provider.clearCalls[credentials.SecretSSHPassword] != 1 {
			t.Fatalf("ssh password was not cleared: state=%+v clears=%d", provider.states[credentials.SecretSSHPassword], provider.clearCalls[credentials.SecretSSHPassword])
		}
	})

	t.Run("disabling tunnel keeps database password", func(t *testing.T) {
		service, store, provider, principal := catalogFixture()
		ssh := &SSHConfig{Enabled: true, Host: "bastion", Port: 22, User: "deploy", AuthMethod: "password", InsecureSkipHostKey: true}
		postgres(store, nil, ssh)
		service.deps.SpecLookup = engine.ConnectionSpecFor
		provider.states[credentials.SecretPassword] = stored
		provider.values[credentials.SecretPassword] = "db-password"
		provider.states[credentials.SecretSSHPassword] = stored
		provider.values[credentials.SecretSSHPassword] = "ssh-password"
		next := *ssh
		next.Enabled = false
		if err := service.Update(context.Background(), principal, ref, UpdateInput{SSHConfig: &next}); err != nil {
			t.Fatalf("Update: %v", err)
		}
		if !provider.states[credentials.SecretPassword].Set {
			t.Fatal("database password was cleared")
		}
	})
}

func TestUpdateAnyTargetChangeRequiresEveryStoredSecretForNonRevealer(t *testing.T) {
	baseParams := engine.Params{"host": "db", "port": "5432", "username": "user", "database": "app"}
	baseSSH := SSHConfig{Enabled: true, Host: "bastion", Port: 22, User: "deploy", AuthMethod: "password", Fingerprint: "SHA256:old"}
	tests := []struct {
		name       string
		currentTLS *TLSConfig
		nextTLS    *TLSConfig
		currentSSH *SSHConfig
		nextSSH    *SSHConfig
		nextParams engine.Params
	}{
		{name: "tls mode", currentTLS: &TLSConfig{Mode: "verify-full"}, nextTLS: &TLSConfig{Mode: "disable"}},
		{name: "tls ca", currentTLS: &TLSConfig{Mode: "verify-ca", CAPEM: "old-ca"}, nextTLS: &TLSConfig{Mode: "verify-ca", CAPEM: "new-ca"}},
		{name: "tls server name", currentTLS: &TLSConfig{Mode: "verify-full", ServerName: "db-a"}, nextTLS: &TLSConfig{Mode: "verify-full", ServerName: "db-b"}},
		{name: "tls client certificate", currentTLS: &TLSConfig{Mode: "verify-full", ClientCertPEM: "old-cert"}, nextTLS: &TLSConfig{Mode: "verify-full", ClientCertPEM: "new-cert"}},
		{name: "database port", nextParams: engine.Params{"host": "db", "port": "5433", "username": "user", "database": "app"}},
		{name: "ssh known hosts", currentSSH: &SSHConfig{Enabled: true, Host: "bastion", Port: 22, User: "deploy", AuthMethod: "password", KnownHostsEntry: "old", InsecureSkipHostKey: true}, nextSSH: &SSHConfig{Enabled: true, Host: "bastion", Port: 22, User: "deploy", AuthMethod: "password", KnownHostsEntry: "new", InsecureSkipHostKey: true}},
		{name: "ssh fingerprint", currentSSH: &baseSSH, nextSSH: &SSHConfig{Enabled: true, Host: "bastion", Port: 22, User: "deploy", AuthMethod: "password", Fingerprint: "SHA256:new"}},
		{name: "ssh insecure skip host key", currentSSH: &baseSSH, nextSSH: &SSHConfig{Enabled: true, Host: "bastion", Port: 22, User: "deploy", AuthMethod: "password", Fingerprint: "SHA256:old", InsecureSkipHostKey: true}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service, store, provider, principal := catalogFixture()
			conn := store.connections[30]
			conn.Driver = "postgres"
			conn.Params, _ = json.Marshal(baseParams)
			if tc.currentTLS != nil {
				conn.TLSConfig, _ = json.Marshal(tc.currentTLS)
			}
			if tc.currentSSH != nil {
				conn.SSHConfig, _ = json.Marshal(tc.currentSSH)
			}
			store.connections[30] = conn
			service.deps.SpecLookup = engine.ConnectionSpecFor
			service.deps.Policy = fakePolicy{permissions: map[string]bool{access.PermConnUpdate: true}}
			for name, value := range map[credentials.SecretName]string{
				credentials.SecretPassword:    "db-password",
				credentials.SecretSSHPassword: "ssh-password",
			} {
				provider.states[name] = credentials.SecretState{Set: true, Source: credentials.SourceStored}
				provider.values[name] = value
			}
			input := UpdateInput{TLSConfig: tc.nextTLS, SSHConfig: tc.nextSSH}
			if tc.nextParams != nil {
				input.Params = tc.nextParams
			}
			err := service.Update(context.Background(), principal, ConnRef{OrgID: 1, WorkspaceID: 10, ConnectionID: 30}, input)
			var validation *ValidationError
			if !errors.As(err, &validation) || validation.Fields["secrets.password"] == "" {
				t.Fatalf("Update error = %v, want database password re-entry", err)
			}
			if provider.revealCalls != 0 {
				t.Fatalf("rejected update revealed %d secrets", provider.revealCalls)
			}
		})
	}
}

func TestUpdateTargetChangesKeepStoredSecretsForRevealingUser(t *testing.T) {
	ref := ConnRef{OrgID: 1, WorkspaceID: 10, ConnectionID: 30}
	stored := credentials.SecretState{Set: true, Source: credentials.SourceStored}

	t.Run("database host", func(t *testing.T) {
		service, store, provider, principal := catalogFixture()
		provider.states[credentials.SecretPassword] = stored
		provider.values[credentials.SecretPassword] = "db-password"
		if err := service.Update(context.Background(), principal, ref, UpdateInput{Params: engine.Params{"host": "corrected"}}); err != nil {
			t.Fatalf("Update: %v", err)
		}
		if string(store.connections[30].Params) != `{"host":"corrected"}` || provider.values[credentials.SecretPassword] != "db-password" || provider.revealCalls != 0 {
			t.Fatalf("params=%s password state=%+v reveals=%d", store.connections[30].Params, provider.states[credentials.SecretPassword], provider.revealCalls)
		}
	})

	t.Run("ssh repoint", func(t *testing.T) {
		service, store, provider, principal := catalogFixture()
		conn := store.connections[30]
		conn.Driver = "postgres"
		conn.Params = json.RawMessage(`{"host":"db","port":"5432","username":"user"}`)
		conn.SSHConfig = json.RawMessage(`{"enabled":true,"host":"bastion-a","port":22,"user":"deploy","auth_method":"password","insecure_skip_host_key":true}`)
		store.connections[30] = conn
		service.deps.SpecLookup = engine.ConnectionSpecFor
		for name, value := range map[credentials.SecretName]string{
			credentials.SecretPassword:    "db-password",
			credentials.SecretSSHPassword: "ssh-password",
		} {
			provider.states[name] = stored
			provider.values[name] = value
		}
		next := &SSHConfig{Enabled: true, Host: "bastion-b", Port: 22, User: "deploy", AuthMethod: "password", InsecureSkipHostKey: true}
		if err := service.Update(context.Background(), principal, ref, UpdateInput{SSHConfig: next}); err != nil {
			t.Fatalf("Update: %v", err)
		}
		var saved SSHConfig
		if err := json.Unmarshal(store.connections[30].SSHConfig, &saved); err != nil || saved.Host != "bastion-b" {
			t.Fatalf("saved SSH config = %+v, %v", saved, err)
		}
		for name, value := range map[credentials.SecretName]string{
			credentials.SecretPassword:    "db-password",
			credentials.SecretSSHPassword: "ssh-password",
		} {
			if !provider.states[name].Set || provider.values[name] != value || len(provider.setCalls[name]) != 0 || provider.clearCalls[name] != 0 {
				t.Fatalf("%s was changed: state=%+v sets=%v clears=%d", name, provider.states[name], provider.setCalls[name], provider.clearCalls[name])
			}
		}
	})

	t.Run("tls server name", func(t *testing.T) {
		service, store, provider, principal := catalogFixture()
		conn := store.connections[30]
		conn.Driver = "postgres"
		conn.Params = json.RawMessage(`{"host":"db","port":"5432","username":"user"}`)
		conn.TLSConfig = json.RawMessage(`{"mode":"verify-full","server_name":"db-a","client_cert_pem":"cert"}`)
		store.connections[30] = conn
		service.deps.SpecLookup = engine.ConnectionSpecFor
		provider.states[credentials.SecretTLSClientKey] = stored
		provider.values[credentials.SecretTLSClientKey] = "client-key"
		next := &TLSConfig{Mode: "verify-full", ServerName: "db-b", ClientCertPEM: "cert"}
		if err := service.Update(context.Background(), principal, ref, UpdateInput{TLSConfig: next}); err != nil {
			t.Fatalf("Update: %v", err)
		}
		var saved TLSConfig
		if err := json.Unmarshal(store.connections[30].TLSConfig, &saved); err != nil || saved.ServerName != "db-b" {
			t.Fatalf("saved TLS config = %+v, %v", saved, err)
		}
		if !provider.states[credentials.SecretTLSClientKey].Set || provider.values[credentials.SecretTLSClientKey] != "client-key" {
			t.Fatal("TLS client key was not preserved")
		}
	})

	t.Run("ssh host key", func(t *testing.T) {
		service, store, provider, principal := catalogFixture()
		conn := store.connections[30]
		conn.Driver = "postgres"
		conn.Params = json.RawMessage(`{"host":"db","port":"5432","username":"user"}`)
		conn.SSHConfig = json.RawMessage(`{"enabled":true,"host":"bastion","port":22,"user":"deploy","auth_method":"password","fingerprint":"SHA256:old"}`)
		store.connections[30] = conn
		service.deps.SpecLookup = engine.ConnectionSpecFor
		provider.states[credentials.SecretSSHPassword] = stored
		provider.values[credentials.SecretSSHPassword] = "ssh-password"
		next := &SSHConfig{Enabled: true, Host: "bastion", Port: 22, User: "deploy", AuthMethod: "password", Fingerprint: "SHA256:new"}
		if err := service.Update(context.Background(), principal, ref, UpdateInput{SSHConfig: next}); err != nil {
			t.Fatalf("Update: %v", err)
		}
		if !provider.states[credentials.SecretSSHPassword].Set || provider.values[credentials.SecretSSHPassword] != "ssh-password" {
			t.Fatal("SSH password was not preserved")
		}
	})
}

func TestUpdateTargetChangesRejectUsersWhoCannotReveal(t *testing.T) {
	type targetCase struct {
		name      string
		configure func(*fakeStore, *fakeCredentials, *Service)
		input     UpdateInput
		field     string
	}
	stored := credentials.SecretState{Set: true, Source: credentials.SourceStored}
	cases := []targetCase{
		{
			name: "database host",
			configure: func(_ *fakeStore, provider *fakeCredentials, _ *Service) {
				provider.states[credentials.SecretPassword] = stored
				provider.values[credentials.SecretPassword] = "db-password"
			},
			input: UpdateInput{Params: engine.Params{"host": "other"}}, field: "secrets.password",
		},
		{
			name: "ssh repoint",
			configure: func(store *fakeStore, provider *fakeCredentials, service *Service) {
				conn := store.connections[30]
				conn.Driver = "postgres"
				conn.Params = json.RawMessage(`{"host":"db","port":"5432","username":"user"}`)
				conn.SSHConfig = json.RawMessage(`{"enabled":true,"host":"bastion-a","port":22,"user":"deploy","auth_method":"password","insecure_skip_host_key":true}`)
				store.connections[30] = conn
				service.deps.SpecLookup = engine.ConnectionSpecFor
				provider.states[credentials.SecretSSHPassword] = stored
				provider.values[credentials.SecretSSHPassword] = "ssh-password"
			},
			input: UpdateInput{SSHConfig: &SSHConfig{Enabled: true, Host: "bastion-b", Port: 22, User: "deploy", AuthMethod: "password", InsecureSkipHostKey: true}}, field: "secrets.ssh_password",
		},
		{
			name: "tls server name",
			configure: func(store *fakeStore, provider *fakeCredentials, service *Service) {
				conn := store.connections[30]
				conn.Driver = "postgres"
				conn.Params = json.RawMessage(`{"host":"db","port":"5432","username":"user"}`)
				conn.TLSConfig = json.RawMessage(`{"mode":"verify-full","server_name":"db-a","client_cert_pem":"cert"}`)
				store.connections[30] = conn
				service.deps.SpecLookup = engine.ConnectionSpecFor
				provider.states[credentials.SecretTLSClientKey] = stored
				provider.values[credentials.SecretTLSClientKey] = "client-key"
			},
			input: UpdateInput{TLSConfig: &TLSConfig{Mode: "verify-full", ServerName: "db-b", ClientCertPEM: "cert"}}, field: "secrets.tls_client_key",
		},
	}
	for _, gate := range []struct {
		name  string
		apply func(*Service)
	}{
		{name: "permission missing", apply: func(service *Service) {
			service.deps.Policy = fakePolicy{permissions: map[string]bool{access.PermConnUpdate: true}}
		}},
		{name: "organization policy denied", apply: func(service *Service) {
			service.deps.RevealPolicy = fakeRevealPolicy{allowed: false}
		}},
	} {
		for _, tc := range cases {
			t.Run(gate.name+"/"+tc.name, func(t *testing.T) {
				service, store, provider, principal := catalogFixture()
				tc.configure(store, provider, service)
				gate.apply(service)
				err := service.Update(context.Background(), principal, ConnRef{OrgID: 1, WorkspaceID: 10, ConnectionID: 30}, tc.input)
				var validation *ValidationError
				if !errors.As(err, &validation) || validation.Fields[tc.field] == "" {
					t.Fatalf("Update error = %v, want field error on %s", err, tc.field)
				}
				if provider.revealCalls != 0 {
					t.Fatalf("denied update revealed %d secrets", provider.revealCalls)
				}
			})
		}
	}
}

func TestUpdateEnablingTunnelAppliesRevealGateToDatabaseAndSSHSecrets(t *testing.T) {
	fixture := func() (*Service, *fakeStore, *fakeCredentials, access.Principal) {
		service, store, provider, principal := catalogFixture()
		conn := store.connections[30]
		conn.Driver = "postgres"
		conn.Params = json.RawMessage(`{"host":"db","port":"5432","username":"user"}`)
		store.connections[30] = conn
		service.deps.SpecLookup = engine.ConnectionSpecFor
		for name, value := range map[credentials.SecretName]string{credentials.SecretPassword: "db-password", credentials.SecretSSHPassword: "ssh-password"} {
			provider.states[name] = credentials.SecretState{Set: true, Source: credentials.SourceStored}
			provider.values[name] = value
		}
		return service, store, provider, principal
	}
	ref := ConnRef{OrgID: 1, WorkspaceID: 10, ConnectionID: 30}
	next := &SSHConfig{Enabled: true, Host: "bastion", Port: 22, User: "deploy", AuthMethod: "password", InsecureSkipHostKey: true}

	t.Run("revealing user", func(t *testing.T) {
		service, store, provider, principal := fixture()
		if err := service.Update(context.Background(), principal, ref, UpdateInput{SSHConfig: next}); err != nil {
			t.Fatalf("Update: %v", err)
		}
		if !provider.states[credentials.SecretPassword].Set || !provider.states[credentials.SecretSSHPassword].Set {
			t.Fatal("stored secrets were not preserved")
		}
		var saved SSHConfig
		if err := json.Unmarshal(store.connections[30].SSHConfig, &saved); err != nil || !saved.Enabled {
			t.Fatalf("saved SSH config = %+v, %v", saved, err)
		}
	})

	t.Run("non-revealing user must re-enter both", func(t *testing.T) {
		service, _, provider, principal := fixture()
		service.deps.Policy = fakePolicy{permissions: map[string]bool{access.PermConnUpdate: true}}
		err := service.Update(context.Background(), principal, ref, UpdateInput{SSHConfig: next})
		var validation *ValidationError
		if !errors.As(err, &validation) || validation.Fields["secrets.password"] == "" {
			t.Fatalf("Update error = %v, want database password error", err)
		}

		service, _, provider, principal = fixture()
		service.deps.Policy = fakePolicy{permissions: map[string]bool{access.PermConnUpdate: true}}
		reentered := "new-db-password"
		err = service.Update(context.Background(), principal, ref, UpdateInput{SSHConfig: next, Secrets: map[credentials.SecretName]*string{credentials.SecretPassword: &reentered}})
		if !errors.As(err, &validation) || validation.Fields["secrets.ssh_password"] == "" {
			t.Fatalf("Update error = %v, want SSH password error", err)
		}
		if provider.revealCalls != 0 || len(provider.setCalls[credentials.SecretPassword]) != 0 {
			t.Fatal("rejected update read or wrote a secret")
		}
	})
}

func TestUpdateRejectsEmptySecretAsTargetGateBypass(t *testing.T) {
	service, store, provider, principal := catalogFixture()
	service.deps.Policy = fakePolicy{permissions: map[string]bool{access.PermConnUpdate: true}}
	provider.states[credentials.SecretPassword] = credentials.SecretState{Set: true, Source: credentials.SourceStored}
	provider.values[credentials.SecretPassword] = "stored-password"
	empty := ""
	err := service.Update(context.Background(), principal, ConnRef{OrgID: 1, WorkspaceID: 10, ConnectionID: 30}, UpdateInput{
		Params: engine.Params{"host": "other"}, Secrets: map[credentials.SecretName]*string{credentials.SecretPassword: &empty},
	})
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Fields["secrets.password"] == "" {
		t.Fatalf("Update error = %v, want empty-secret validation error", err)
	}
	if string(store.connections[30].Params) != `{"host":"old"}` || provider.values[credentials.SecretPassword] != "stored-password" || len(provider.setCalls[credentials.SecretPassword]) != 0 || provider.revealCalls != 0 {
		t.Fatalf("rejected bypass changed or revealed state: params=%s state=%+v sets=%v reveals=%d", store.connections[30].Params, provider.states[credentials.SecretPassword], provider.setCalls[credentials.SecretPassword], provider.revealCalls)
	}
	if strings.Contains(err.Error(), "stored-password") {
		t.Fatal("validation error leaked the stored secret")
	}
}

func TestUpdateDesktopStyleAlwaysAllowPolicyKeepsStoredSecret(t *testing.T) {
	service, store, provider, principal := catalogFixture()
	policy := &countingRevealPolicy{}
	service.deps.RevealPolicy = policy
	provider.states[credentials.SecretPassword] = credentials.SecretState{Set: true, Source: credentials.SourceStored}
	provider.values[credentials.SecretPassword] = "stored-password"
	if err := service.Update(context.Background(), principal, ConnRef{OrgID: 1, WorkspaceID: 10, ConnectionID: 30}, UpdateInput{Params: engine.Params{"host": "corrected"}}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if policy.calls != 1 || provider.values[credentials.SecretPassword] != "stored-password" || string(store.connections[30].Params) != `{"host":"corrected"}` {
		t.Fatalf("policy calls=%d params=%s secret=%+v", policy.calls, store.connections[30].Params, provider.states[credentials.SecretPassword])
	}
}

func TestStoredSSHDefaultsAreCanonicalForUpdateAndTest(t *testing.T) {
	fixture := func() (*Service, *fakeCredentials, access.Principal) {
		service, store, provider, principal := catalogFixture()
		conn := store.connections[30]
		conn.Driver = "postgres"
		conn.Params = json.RawMessage(`{"host":"db","port":"5432","username":"user"}`)
		conn.SSHConfig = json.RawMessage(`{"enabled":true,"host":"bastion","port":0,"user":"deploy","auth_method":"","insecure_skip_host_key":true}`)
		store.connections[30] = conn
		service.deps.SpecLookup = engine.ConnectionSpecFor
		for _, name := range []credentials.SecretName{credentials.SecretPassword, credentials.SecretSSHPassword} {
			provider.states[name] = credentials.SecretState{Set: true, Source: credentials.SourceStored}
			provider.values[name] = string(name) + "-value"
		}
		return service, provider, principal
	}
	normalized := &SSHConfig{Enabled: true, Host: "bastion", Port: 22, User: "deploy", AuthMethod: "password", InsecureSkipHostKey: true}

	t.Run("update", func(t *testing.T) {
		service, _, principal := fixture()
		ref := ConnRef{OrgID: 1, WorkspaceID: 10, ConnectionID: 30}
		if err := service.Update(context.Background(), principal, ref, UpdateInput{SSHConfig: normalized}); err != nil {
			t.Fatalf("Update: %v", err)
		}
	})

	t.Run("test", func(t *testing.T) {
		service, provider, principal := fixture()
		prober := &captureProber{}
		service.deps.Prober = prober
		result, err := service.Test(context.Background(), principal, TestInput{OrgID: 1, WorkspaceID: 10, ConnectionID: connIDPtr(30), Driver: "postgres", Params: engine.Params{"host": "db", "port": "5432", "username": "user"}, SSHConfig: normalized})
		if err != nil || !result.OK || !prober.probed || provider.revealCalls != 2 {
			t.Fatalf("Test = %+v, %v reveals=%d", result, err, provider.revealCalls)
		}
	})
}

func TestTestConnectionIDSecretReuseIsScopedByTarget(t *testing.T) {
	stored := credentials.SecretState{Set: true, Source: credentials.SourceStored}

	t.Run("tls downgrade audits database password reuse", func(t *testing.T) {
		service, store, provider, principal := catalogFixture()
		recorder := &auditRecorder{}
		service.deps.Audit = recorder
		conn := store.connections[30]
		conn.Driver = "postgres"
		conn.Params = json.RawMessage(`{"host":"db","port":"5432","username":"user"}`)
		conn.TLSConfig = json.RawMessage(`{"mode":"verify-full"}`)
		store.connections[30] = conn
		service.deps.SpecLookup = engine.ConnectionSpecFor
		provider.states[credentials.SecretPassword] = stored
		provider.values[credentials.SecretPassword] = "db-password"
		prober := &captureProber{}
		service.deps.Prober = prober
		result, err := service.Test(context.Background(), principal, TestInput{OrgID: 1, WorkspaceID: 10, ConnectionID: connIDPtr(30), Driver: "postgres", Params: engine.Params{"host": "db", "port": "5432", "username": "user"}, TLSConfig: &TLSConfig{Mode: "disable"}})
		if err != nil || !result.OK || !prober.probed || provider.revealCalls != 1 || len(recorder.events) != 1 || recorder.events[0].Metadata["secret_name"] != "password" || recorder.events[0].Metadata["via"] != "test" {
			t.Fatalf("Test = %+v, %v reveals=%d events=%+v", result, err, provider.revealCalls, recorder.events)
		}
	})

	t.Run("tls server name change does not reuse client key", func(t *testing.T) {
		service, store, provider, principal := catalogFixture()
		service.deps.Policy = fakePolicy{permissions: map[string]bool{access.PermConnCreate: true, access.PermConnRead: true}}
		conn := store.connections[30]
		conn.Driver = "postgres"
		conn.Params = json.RawMessage(`{"host":"db","port":"5432","username":"user"}`)
		conn.TLSConfig = json.RawMessage(`{"mode":"verify-full","server_name":"db-a","client_cert_pem":"cert"}`)
		store.connections[30] = conn
		service.deps.SpecLookup = engine.ConnectionSpecFor
		provider.states[credentials.SecretTLSClientKey] = stored
		provider.values[credentials.SecretTLSClientKey] = "client-key"
		service.deps.Prober = &captureProber{}
		_, err := service.Test(context.Background(), principal, TestInput{OrgID: 1, WorkspaceID: 10, ConnectionID: connIDPtr(30), Driver: "postgres", Params: engine.Params{"host": "db", "port": "5432", "username": "user"}, TLSConfig: &TLSConfig{Mode: "verify-full", ServerName: "db-b", ClientCertPEM: "cert"}})
		var validation *ValidationError
		if !errors.As(err, &validation) || validation.Fields["secrets.tls_client_key"] == "" || provider.revealCalls != 0 {
			t.Fatalf("Test error = %v reveals=%d", err, provider.revealCalls)
		}
	})
}

func TestTestConnectionIDAnyTargetChangeRejectsStoredSecretsForNonRevealer(t *testing.T) {
	baseParams := engine.Params{"host": "db", "port": "5432", "username": "user", "database": "app"}
	baseSSH := SSHConfig{Enabled: true, Host: "bastion", Port: 22, User: "deploy", AuthMethod: "password", Fingerprint: "SHA256:old"}
	tests := []struct {
		name       string
		currentTLS *TLSConfig
		nextTLS    *TLSConfig
		currentSSH *SSHConfig
		nextSSH    *SSHConfig
		nextParams engine.Params
	}{
		{name: "tls mode", currentTLS: &TLSConfig{Mode: "verify-full"}, nextTLS: &TLSConfig{Mode: "disable"}},
		{name: "tls ca", currentTLS: &TLSConfig{Mode: "verify-ca", CAPEM: "old-ca"}, nextTLS: &TLSConfig{Mode: "verify-ca", CAPEM: "new-ca"}},
		{name: "tls server name", currentTLS: &TLSConfig{Mode: "verify-full", ServerName: "db-a"}, nextTLS: &TLSConfig{Mode: "verify-full", ServerName: "db-b"}},
		{name: "database port", nextParams: engine.Params{"host": "db", "port": "5433", "username": "user", "database": "app"}},
		{name: "ssh known hosts", currentSSH: &SSHConfig{Enabled: true, Host: "bastion", Port: 22, User: "deploy", AuthMethod: "password", KnownHostsEntry: "old", InsecureSkipHostKey: true}, nextSSH: &SSHConfig{Enabled: true, Host: "bastion", Port: 22, User: "deploy", AuthMethod: "password", KnownHostsEntry: "new", InsecureSkipHostKey: true}},
		{name: "ssh fingerprint", currentSSH: &baseSSH, nextSSH: &SSHConfig{Enabled: true, Host: "bastion", Port: 22, User: "deploy", AuthMethod: "password", Fingerprint: "SHA256:new"}},
		{name: "ssh insecure skip host key", currentSSH: &baseSSH, nextSSH: &SSHConfig{Enabled: true, Host: "bastion", Port: 22, User: "deploy", AuthMethod: "password", Fingerprint: "SHA256:old", InsecureSkipHostKey: true}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service, store, provider, principal := catalogFixture()
			recorder := &auditRecorder{}
			service.deps.Audit = recorder
			service.deps.Policy = fakePolicy{permissions: map[string]bool{access.PermConnCreate: true, access.PermConnRead: true}}
			conn := store.connections[30]
			conn.Driver = "postgres"
			conn.Params, _ = json.Marshal(baseParams)
			if tc.currentTLS != nil {
				conn.TLSConfig, _ = json.Marshal(tc.currentTLS)
			}
			if tc.currentSSH != nil {
				conn.SSHConfig, _ = json.Marshal(tc.currentSSH)
			}
			store.connections[30] = conn
			service.deps.SpecLookup = engine.ConnectionSpecFor
			provider.states[credentials.SecretPassword] = credentials.SecretState{Set: true, Source: credentials.SourceStored}
			provider.values[credentials.SecretPassword] = "db-password"
			prober := &captureProber{}
			service.deps.Prober = prober
			params := baseParams
			if tc.nextParams != nil {
				params = tc.nextParams
			}
			_, err := service.Test(context.Background(), principal, TestInput{OrgID: 1, WorkspaceID: 10, ConnectionID: connIDPtr(30), Driver: "postgres", Params: params, TLSConfig: tc.nextTLS, SSHConfig: tc.nextSSH})
			var validation *ValidationError
			if !errors.As(err, &validation) || validation.Fields["secrets.password"] == "" {
				t.Fatalf("Test error = %v, want database password re-entry", err)
			}
			if provider.revealCalls != 0 || len(recorder.events) != 0 || prober.probed {
				t.Fatalf("rejected test side effects: reveals=%d audits=%d probed=%v", provider.revealCalls, len(recorder.events), prober.probed)
			}
		})
	}
}

func TestTestConnectionIDChangedTargetAuditsEveryStoredSecretUsed(t *testing.T) {
	service, store, provider, principal := catalogFixture()
	recorder := &auditRecorder{}
	service.deps.Audit = recorder
	conn := store.connections[30]
	conn.Driver = "postgres"
	conn.Params = json.RawMessage(`{"host":"db","port":"5432","username":"user"}`)
	conn.TLSConfig = json.RawMessage(`{"mode":"verify-ca","ca_pem":"old-ca","client_cert_pem":"cert"}`)
	store.connections[30] = conn
	service.deps.SpecLookup = engine.ConnectionSpecFor
	for name, value := range map[credentials.SecretName]string{
		credentials.SecretPassword:     "db-password",
		credentials.SecretTLSClientKey: "client-key",
	} {
		provider.states[name] = credentials.SecretState{Set: true, Source: credentials.SourceStored}
		provider.values[name] = value
	}
	prober := &captureProber{}
	service.deps.Prober = prober

	result, err := service.Test(context.Background(), principal, TestInput{
		OrgID: 1, WorkspaceID: 10, ConnectionID: connIDPtr(30), Driver: "postgres",
		Params:    engine.Params{"host": "db", "port": "5432", "username": "user"},
		TLSConfig: &TLSConfig{Mode: "verify-ca", CAPEM: "new-ca", ClientCertPEM: "cert"},
	})
	if err != nil || !result.OK || !prober.probed || provider.revealCalls != 2 || len(recorder.events) != 2 {
		t.Fatalf("Test = %+v, %v reveals=%d events=%+v", result, err, provider.revealCalls, recorder.events)
	}
	wantNames := map[string]bool{"password": true, "tls_client_key": true}
	for _, event := range recorder.events {
		if !wantNames[event.Metadata["secret_name"]] || event.Metadata["via"] != "test" {
			t.Fatalf("audit event = %+v", event)
		}
		delete(wantNames, event.Metadata["secret_name"])
	}
	if len(wantNames) != 0 {
		t.Fatalf("missing audit events for %v", wantNames)
	}
}
