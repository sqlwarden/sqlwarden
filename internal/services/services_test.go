package services

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/credentials"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/encrypt"
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/execution"
	"github.com/sqlwarden/internal/settings"
	"github.com/sqlwarden/internal/smtp"
)

type denyReveal struct{}

func (denyReveal) Allowed(context.Context, credentials.OrgRef, access.Principal) (bool, error) {
	return false, nil
}

type recordingPolicy struct {
	mu    sync.Mutex
	calls []string
}

func (p *recordingPolicy) Can(_ context.Context, _ int64, _ int64, _, _ string, _ int64, permission string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, permission)
	return false
}

func (p *recordingPolicy) EffectivePermissions(context.Context, int64, int64, string, string, int64) ([]string, error) {
	return nil, nil
}

func newTestDeps(t *testing.T) Deps {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := database.New("sqlite", filepath.Join(t.TempDir(), "services.db"), logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.MigrateUp(); err != nil {
		t.Fatal(err)
	}
	enforcer, err := access.New(db.DB)
	if err != nil {
		t.Fatal(err)
	}
	keyring, err := encrypt.NewKeyring("test-encryption-key-32bytes!!!!!")
	if err != nil {
		t.Fatal(err)
	}
	core := credentials.NewEncryptedColumnProvider(db, credentials.NewKeyringSealer(keyring), engine.ConnectionSpecFor)
	runtime := execution.NewLocal(execution.LocalConfig{
		Credentials: core,
		Policy:      settings.NewTargetPolicy(db),
		IdleTimeout: time.Minute,
		Logger:      logger,
	})
	t.Cleanup(runtime.Shutdown)
	return Deps{
		DB:               db,
		Enforcer:         enforcer,
		Runtime:          runtime,
		Logger:           logger,
		Credentials:      core,
		CredentialWriter: core,
		RevealPolicy:     denyReveal{},
		SecretSealer:     keyring,
		Keyring:          keyring,
	}
}

func TestInfraSettingsAppliersInstallTheMailerServicesSendThrough(t *testing.T) {
	deps := newTestDeps(t)
	ctx := context.Background()
	if err := settings.InitializeInstanceBaseURL(ctx, deps.DB, "https://sqlwarden.example.com"); err != nil {
		t.Fatal(err)
	}
	svc, infra, err := Build(deps)
	if err != nil {
		t.Fatal(err)
	}
	in, err := svc.Settings.Instance(ctx)
	if err != nil {
		t.Fatal(err)
	}
	in.SMTPEnabled = false
	if infra.Mailer.Current() != nil {
		t.Fatal("a mailer was installed before any settings were applied")
	}
	for _, apply := range infra.SettingsAppliers() {
		if err := apply(ctx, in); err != nil {
			t.Fatal(err)
		}
	}
	if infra.Mailer.Current() == nil {
		t.Fatal("settings appliers did not install a mailer")
	}
	assert.ErrorIs(t, svc.Mailer.Send("someone@example.com", nil, "error-notification.tmpl"), smtp.ErrDisabled)
}

func TestBuildRequiresSecretSealer(t *testing.T) {
	deps := newTestDeps(t)
	deps.SecretSealer = nil
	if _, _, err := Build(deps); err == nil {
		t.Fatal("Build without a secret sealer returned no error")
	}
}

func TestBuildRequiresKeyring(t *testing.T) {
	deps := newTestDeps(t)
	deps.Keyring = nil
	if _, _, err := Build(deps); err == nil {
		t.Fatal("Build without a keyring returned no error")
	}
}

func TestBuildProjectsEffectiveSettingsOntoServicePolicies(t *testing.T) {
	deps := newTestDeps(t)
	svc, _, err := Build(deps)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	instance, _, err := deps.DB.GetInstanceSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	instance.BaseURL = "https://sqlwarden.example.com"
	instance.QueryMaxResultRows = 1000
	instance.ExportsSyncMaxBytes = 4096
	if _, err := deps.DB.UpsertInstanceSettings(ctx, instance); err != nil {
		t.Fatal(err)
	}
	org, err := deps.DB.InsertOrg(ctx, "acme", "Acme")
	if err != nil {
		t.Fatal(err)
	}
	rows, syncBytes := 25, int64(512)
	if _, err := deps.DB.UpsertOrganizationRuntimeSettings(ctx, database.OrganizationRuntimeSettings{
		OrgID: org.ID, QueryMaxResultRows: &rows, ExportsSyncMaxBytes: &syncBytes,
	}); err != nil {
		t.Fatal(err)
	}
	ws := access.Workspace{OwnerType: "org", OwnerID: org.ID}

	queryPolicy, err := queryPolicies{settings: svc.Settings}.ForWorkspace(ctx, ws)
	assert.Nil(t, err)
	assert.Equal(t, queryPolicy.MaxRows, 25)
	assert.Equal(t, queryPolicy.CursorPageSize, instance.QueryCursorPageSize)

	exportPolicy, err := exportPolicies{settings: svc.Settings}.ForWorkspace(ctx, ws)
	assert.Nil(t, err)
	assert.Equal(t, exportPolicy.SyncMaxBytes, int64(512))
}

func accountPrincipal(id int64) access.Principal {
	return access.Principal{Subject: access.SubjectRef{Kind: access.SubjectAccount, ID: id}}
}

func TestBuildRequiresDatabase(t *testing.T) {
	deps := newTestDeps(t)
	deps.DB = nil
	if _, _, err := Build(deps); err == nil {
		t.Fatal("Build without a database returned no error")
	}
}

func TestBuildWiresRequirerOverDatabaseDirectory(t *testing.T) {
	deps := newTestDeps(t)
	_, infra, err := Build(deps)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	org, err := deps.DB.InsertOrg(ctx, "acme", "Acme")
	if err != nil {
		t.Fatal(err)
	}
	member, err := deps.DB.InsertAccount(ctx, "member@example.com", "Member", nil)
	if err != nil {
		t.Fatal(err)
	}
	outsider, err := deps.DB.InsertAccount(ctx, "outsider@example.com", "Outsider", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := deps.DB.AddOrgMember(ctx, org.ID, member.ID); err != nil {
		t.Fatal(err)
	}

	got, err := infra.Requirer.Org(ctx, accountPrincipal(member.ID), org.ID)
	assert.Nil(t, err)
	assert.Equal(t, got.Slug, "acme")

	_, err = infra.Requirer.Org(ctx, accountPrincipal(outsider.ID), org.ID)
	assert.ErrorIs(t, err, access.ErrForbidden)
}

func TestBuildPassesPolicyToRequirer(t *testing.T) {
	deps := newTestDeps(t)
	policy := &recordingPolicy{}
	deps.Policy = policy
	_, infra, err := Build(deps)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	org, err := deps.DB.InsertOrg(ctx, "acme", "Acme")
	if err != nil {
		t.Fatal(err)
	}
	account, err := deps.DB.InsertAccount(ctx, "member@example.com", "Member", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := deps.DB.AddOrgMember(ctx, org.ID, account.ID); err != nil {
		t.Fatal(err)
	}
	ws, err := deps.DB.InsertWorkspace(ctx, &org.ID, "org", org.ID, "Engineering", "")
	if err != nil {
		t.Fatal(err)
	}

	_, err = infra.Requirer.Workspace(ctx, accountPrincipal(account.ID), org.ID, ws.ID, "workspace:read")
	assert.ErrorIs(t, err, access.ErrForbidden)
	assert.Equal(t, len(policy.calls), 1)
	assert.Equal(t, policy.calls[0], "workspace:read")
	assert.Equal(t, infra.Policy, access.PolicyEvaluator(policy))
}

type recordingDirectoryStore struct {
	getOrgID                int64
	isOrgMemberArgs         [2]int64
	isEffectiveMemberArgs   [3]int64
	getWorkspaceID          int64
	getConnectionID         int64
	workspaceOrganizationID int64
}

func (s *recordingDirectoryStore) GetOrg(_ context.Context, id int64) (database.Organization, bool, error) {
	s.getOrgID = id
	return database.Organization{ID: 11, Slug: "acme", Name: "Acme"}, true, nil
}

func (s *recordingDirectoryStore) IsOrgMember(_ context.Context, orgID, accountID int64) (bool, error) {
	s.isOrgMemberArgs = [2]int64{orgID, accountID}
	return true, nil
}

func (s *recordingDirectoryStore) IsEffectiveWorkspaceMember(_ context.Context, orgID, workspaceID, accountID int64) (bool, error) {
	s.isEffectiveMemberArgs = [3]int64{orgID, workspaceID, accountID}
	return true, nil
}

func (s *recordingDirectoryStore) GetWorkspace(_ context.Context, id int64) (database.Workspace, bool, error) {
	s.getWorkspaceID = id
	return database.Workspace{
		ID:        22,
		OrgID:     &s.workspaceOrganizationID,
		OwnerType: "org",
		OwnerID:   11,
		Name:      "Engineering",
	}, true, nil
}

func (s *recordingDirectoryStore) GetConnection(_ context.Context, id int64) (database.Connection, bool, error) {
	s.getConnectionID = id
	return database.Connection{
		ID:            33,
		WorkspaceID:   22,
		EnvironmentID: 44,
		Name:          "Primary",
		Driver:        "postgres",
	}, true, nil
}

func TestDirectoryAdapterForwards(t *testing.T) {
	store := &recordingDirectoryStore{workspaceOrganizationID: 11}
	adapter := directoryAdapter{store: store}
	ctx := context.Background()

	org, found, err := adapter.GetOrg(ctx, 101)
	assert.Nil(t, err)
	assert.True(t, found)
	assert.Equal(t, org, access.Org{ID: 11, Slug: "acme", Name: "Acme"})

	member, err := adapter.IsOrgMember(ctx, 102, 103)
	assert.Nil(t, err)
	assert.True(t, member)

	effectiveMember, err := adapter.IsEffectiveWorkspaceMember(ctx, 104, 105, 106)
	assert.Nil(t, err)
	assert.True(t, effectiveMember)

	workspace, found, err := adapter.GetWorkspace(ctx, 107)
	assert.Nil(t, err)
	assert.True(t, found)
	assert.Equal(t, workspace, access.Workspace{
		ID:        22,
		OrgID:     &store.workspaceOrganizationID,
		OwnerType: "org",
		OwnerID:   11,
		Name:      "Engineering",
	})

	connection, found, err := adapter.GetConnection(ctx, 108)
	assert.Nil(t, err)
	assert.True(t, found)
	assert.Equal(t, connection, access.Connection{
		ID:            33,
		WorkspaceID:   22,
		EnvironmentID: 44,
		Name:          "Primary",
		Driver:        "postgres",
	})

	assert.Equal(t, store.getOrgID, int64(101))
	assert.Equal(t, store.isOrgMemberArgs, [2]int64{102, 103})
	assert.Equal(t, store.isEffectiveMemberArgs, [3]int64{104, 105, 106})
	assert.Equal(t, store.getWorkspaceID, int64(107))
	assert.Equal(t, store.getConnectionID, int64(108))
}

func TestDirectoryAdapterRejectsNilStore(t *testing.T) {
	adapter := directoryAdapter{}
	ctx := context.Background()

	_, _, err := adapter.GetOrg(ctx, 1)
	assert.ErrorIs(t, err, errDirectoryStoreNotConfigured)
	_, err = adapter.IsOrgMember(ctx, 1, 2)
	assert.ErrorIs(t, err, errDirectoryStoreNotConfigured)
	_, err = adapter.IsEffectiveWorkspaceMember(ctx, 1, 2, 3)
	assert.ErrorIs(t, err, errDirectoryStoreNotConfigured)
	_, _, err = adapter.GetWorkspace(ctx, 1)
	assert.ErrorIs(t, err, errDirectoryStoreNotConfigured)
	_, _, err = adapter.GetConnection(ctx, 1)
	assert.ErrorIs(t, err, errDirectoryStoreNotConfigured)
}
