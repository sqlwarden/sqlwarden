package settings_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/access/accesstest"
	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/settings"
)

const (
	testOrg     int64 = 1
	foreignOrg  int64 = 2
	adminID     int64 = 5
	writerID    int64 = 7
	readerID    int64 = 8
	nonMemberID int64 = 9
)

type fakeStore struct {
	instance      database.InstanceSettings
	instanceFound bool
	instanceErr   error
	orgs          map[int64]database.OrganizationRuntimeSettings
	anyAdmin      bool
	historyRows   bool
	favoriteRows  bool

	writes           int
	historyCleared   []int64
	favoritesCleared []int64
	seededBaseURL    string
}

func newFakeStore() *fakeStore {
	in := database.DefaultInstanceSettings()
	in.BaseURL = "https://sqlwarden.example.com"
	return &fakeStore{
		instance:      in,
		instanceFound: true,
		orgs:          map[int64]database.OrganizationRuntimeSettings{},
	}
}

func (s *fakeStore) GetInstanceSettings(context.Context) (database.InstanceSettings, bool, error) {
	return s.instance, s.instanceFound, s.instanceErr
}

func (s *fakeStore) UpsertInstanceSettings(_ context.Context, in database.InstanceSettings) (database.InstanceSettings, error) {
	s.writes++
	s.instance = in
	s.instanceFound = true
	return in, nil
}

func (s *fakeStore) GetOrganizationRuntimeSettings(_ context.Context, orgID int64) (database.OrganizationRuntimeSettings, bool, error) {
	row, ok := s.orgs[orgID]
	return row, ok, nil
}

func (s *fakeStore) UpsertOrganizationRuntimeSettings(_ context.Context, row database.OrganizationRuntimeSettings) (database.OrganizationRuntimeSettings, error) {
	s.writes++
	s.orgs[row.OrgID] = row
	return row, nil
}

func (s *fakeStore) ClearQueryHistoryForOrg(_ context.Context, orgID int64) error {
	s.writes++
	s.historyCleared = append(s.historyCleared, orgID)
	return nil
}

func (s *fakeStore) ClearQueryFavoritesForOrg(_ context.Context, orgID int64) error {
	s.writes++
	s.favoritesCleared = append(s.favoritesCleared, orgID)
	return nil
}

func (s *fakeStore) QueryHistoryHasRowsForOrg(context.Context, int64) (bool, error) {
	return s.historyRows, nil
}

func (s *fakeStore) QueryFavoritesHasRowsForOrg(context.Context, int64) (bool, error) {
	return s.favoriteRows, nil
}

func (s *fakeStore) HasAnyInstanceAdmin(context.Context) (bool, error) {
	return s.anyAdmin, nil
}

func (s *fakeStore) InitializeInstanceBaseURL(_ context.Context, baseURL string) (database.InstanceSettings, error) {
	s.seededBaseURL = baseURL
	s.instance.BaseURL = baseURL
	return s.instance, nil
}

type fakeDirectory struct{}

func (fakeDirectory) GetOrg(_ context.Context, id int64) (access.Org, bool, error) {
	return access.Org{ID: id}, id == testOrg, nil
}

func (fakeDirectory) IsOrgMember(_ context.Context, orgID, accountID int64) (bool, error) {
	return accountID == writerID || accountID == readerID, nil
}

func (fakeDirectory) IsEffectiveWorkspaceMember(context.Context, int64, int64, int64) (bool, error) {
	return false, nil
}

func (fakeDirectory) GetWorkspace(context.Context, int64) (access.Workspace, bool, error) {
	return access.Workspace{}, false, nil
}

func (fakeDirectory) GetConnection(context.Context, int64) (access.Connection, bool, error) {
	return access.Connection{}, false, nil
}

type fakePolicy struct{}

func (fakePolicy) Can(_ context.Context, accountID, orgID int64, ownerType, resourceType string, resourceID int64, permission string) bool {
	if ownerType != "org" || resourceType != "org" || resourceID != orgID {
		return false
	}
	switch accountID {
	case writerID:
		return permission == access.PermOrgRead || permission == access.PermOrgWrite
	case readerID:
		return permission == access.PermOrgRead
	}
	return false
}

func (fakePolicy) EffectivePermissions(context.Context, int64, int64, string, string, int64) ([]string, error) {
	return nil, nil
}

type fakeSealer struct{}

func (fakeSealer) Encrypt(plaintext string) (string, error) { return "sealed:" + plaintext, nil }

type fakeRotator struct {
	fakeSealer
	stale bool
}

func (r fakeRotator) Decrypt(ciphertext string) (string, error) {
	return strings.TrimPrefix(strings.TrimPrefix(ciphertext, "sealed:"), "old:"), nil
}

func (r fakeRotator) NeedsRotation(string) bool { return r.stale }

func newService(store *fakeStore) *settings.Service {
	return settings.New(settings.Deps{
		Store:    store,
		Requirer: access.NewRequirer(fakeDirectory{}, fakePolicy{}),
		Sealer:   fakeSealer{},
	})
}

func account(id int64) access.Principal {
	return access.Principal{Subject: access.SubjectRef{Kind: access.SubjectAccount, ID: id}}
}

func instanceAdmin(id int64) access.Principal {
	p := account(id)
	p.InstanceAdmin = true
	return p
}

func ptr[T any](v T) *T { return &v }

func set[T any](v T) settings.Patch[T] { return settings.Patch[T]{Set: true, Value: &v} }

func TestInstanceSettingsMethodsRequireInstanceAdmin(t *testing.T) {
	ctx := context.Background()
	nonAccount := access.Principal{Subject: access.SubjectRef{Kind: access.SubjectServiceAccount, ID: adminID}, InstanceAdmin: true}
	for name, p := range map[string]access.Principal{"member": account(writerID), "unflagged": account(adminID), "non-account": nonAccount} {
		t.Run(name, func(t *testing.T) {
			store := newFakeStore()
			svc := newService(store)
			accesstest.AssertForbidden(t, accesstest.Err(svc.GetInstance(ctx, p)), "GetInstance")
			accesstest.AssertForbidden(t, accesstest.Err(svc.UpdateInstance(ctx, p, settings.UpdateInstanceInput{InstanceName: ptr("x")})), "UpdateInstance")
			if store.writes != 0 {
				t.Fatalf("denied calls wrote %d times", store.writes)
			}
		})
	}
}

func TestInstanceSettingsAdmitInstanceAdmins(t *testing.T) {
	ctx := context.Background()
	svc := newService(newFakeStore())
	if _, err := svc.GetInstance(ctx, instanceAdmin(nonMemberID)); err != nil {
		t.Fatalf("flagged admin: %v", err)
	}
}

func TestOrgSettingsMethodsDenyBeforeStore(t *testing.T) {
	ctx := context.Background()
	update := settings.UpdateOrgInput{QueryMaxResultRows: set(10)}
	cases := []struct {
		name    string
		p       access.Principal
		orgID   int64
		want    error
		allowed map[string]bool
	}{
		{name: "non-member", p: account(nonMemberID), orgID: testOrg, want: access.ErrForbidden},
		{name: "foreign org", p: account(writerID), orgID: foreignOrg, want: access.ErrNotFound},
		{name: "reader writes", p: account(readerID), orgID: testOrg, want: access.ErrForbidden, allowed: map[string]bool{"GetOrg": true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeStore()
			svc := newService(store)
			calls := map[string]error{
				"GetOrg":            accesstest.Err(svc.GetOrg(ctx, tc.p, tc.orgID)),
				"UpdateOrg":         accesstest.Err(svc.UpdateOrg(ctx, tc.p, tc.orgID, update)),
				"PurgeOrgHistory":   svc.PurgeOrgHistory(ctx, tc.p, tc.orgID),
				"PurgeOrgFavorites": svc.PurgeOrgFavorites(ctx, tc.p, tc.orgID),
			}
			for name, err := range calls {
				if tc.allowed[name] {
					if err != nil {
						t.Fatalf("%s error = %v", name, err)
					}
					continue
				}
				accesstest.AssertDenies(t, err, tc.want, name)
			}
			if store.writes != 0 {
				t.Fatalf("denied calls wrote %d times", store.writes)
			}
		})
	}
}

func TestInstanceUnavailable(t *testing.T) {
	ctx := context.Background()
	missing := newFakeStore()
	missing.instanceFound = false
	failing := newFakeStore()
	failing.instanceErr = errors.New("boom")
	invalid := newFakeStore()
	invalid.instance.BaseURL = ""
	for name, store := range map[string]*fakeStore{"missing": missing, "failing": failing, "invalid": invalid} {
		if _, err := newService(store).Instance(ctx); !errors.Is(err, settings.ErrUnavailable) {
			t.Fatalf("%s: error = %v, want ErrUnavailable", name, err)
		}
	}
	var nilService *settings.Service
	if _, err := nilService.Instance(ctx); !errors.Is(err, settings.ErrUnavailable) {
		t.Fatalf("nil service: error = %v, want ErrUnavailable", err)
	}
}

func TestEffectiveForOrgOnlyNarrows(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	store.instance.QueryMaxResultRows = 100
	store.instance.QueryHistoryRetentionCount = 50
	store.instance.ExportsBackgroundMaxBytes = 1000
	store.instance.FileRevisionsEnabled = false
	store.orgs[testOrg] = database.OrganizationRuntimeSettings{
		OrgID:                      testOrg,
		QueryMaxResultRows:         ptr(500),
		QueryHistoryRetentionCount: ptr(80),
		ExportsBackgroundMaxBytes:  ptr(int64(0)),
		FileRevisionsEnabled:       ptr(true),
		QueryHistoryMode:           ptr(settings.HistoryModeLocal),
	}
	orgID := testOrg
	got, err := newService(store).EffectiveForOrg(ctx, &orgID)
	if err != nil {
		t.Fatal(err)
	}
	if got.QueryMaxResultRows != 100 || got.QueryHistoryRetentionCount != 50 || got.ExportsBackgroundMaxBytes != 1000 || got.FileRevisionsEnabled {
		t.Fatalf("widening overrides applied: %+v", got)
	}
	if got.QueryHistoryMode != settings.HistoryModeLocal {
		t.Fatalf("history mode = %q, want local", got.QueryHistoryMode)
	}

	store.orgs[testOrg] = database.OrganizationRuntimeSettings{
		OrgID:                      testOrg,
		QueryMaxResultRows:         ptr(25),
		QueryHistoryRetentionCount: ptr(10),
	}
	got, err = newService(store).EffectiveForOrg(ctx, &orgID)
	if err != nil {
		t.Fatal(err)
	}
	if got.QueryMaxResultRows != 25 || got.QueryHistoryRetentionCount != 10 {
		t.Fatalf("narrowing overrides not applied: %+v", got)
	}
}

func TestEffectiveForOrgKeepsInstanceHistoryOff(t *testing.T) {
	store := newFakeStore()
	store.instance.QueryHistoryMode = settings.HistoryModeOff
	store.instance.QueryFavoritesMode = settings.HistoryModeOff
	store.orgs[testOrg] = database.OrganizationRuntimeSettings{
		OrgID:              testOrg,
		QueryHistoryMode:   ptr(settings.HistoryModeBackend),
		QueryFavoritesMode: ptr(settings.HistoryModeBackend),
	}
	orgID := testOrg
	got, err := newService(store).EffectiveForOrg(context.Background(), &orgID)
	if err != nil {
		t.Fatal(err)
	}
	if got.QueryHistoryMode != settings.HistoryModeOff || got.QueryFavoritesMode != settings.HistoryModeOff {
		t.Fatalf("modes = %q/%q, want off/off", got.QueryHistoryMode, got.QueryFavoritesMode)
	}
}

func TestEffectiveForWorkspaceAppliesOwningOrgOnly(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	store.instance.QueryMaxResultRows = 100
	store.orgs[testOrg] = database.OrganizationRuntimeSettings{OrgID: testOrg, QueryMaxResultRows: ptr(25)}
	svc := newService(store)
	orgID := testOrg
	got, err := svc.EffectiveForWorkspace(ctx, access.Workspace{ID: 1, OrgID: &orgID, OwnerType: "org", OwnerID: testOrg})
	if err != nil {
		t.Fatal(err)
	}
	if got.QueryMaxResultRows != 25 {
		t.Fatalf("org workspace rows = %d, want 25", got.QueryMaxResultRows)
	}
	got, err = svc.EffectiveForWorkspace(ctx, access.Workspace{ID: 2, OwnerType: "account", OwnerID: testOrg})
	if err != nil {
		t.Fatal(err)
	}
	if got.QueryMaxResultRows != 100 {
		t.Fatalf("personal workspace rows = %d, want 100", got.QueryMaxResultRows)
	}
}

func TestValidate(t *testing.T) {
	valid := settings.Instance(newFakeStore().instance)
	if err := settings.Validate(valid); err != nil {
		t.Fatalf("valid settings: %v", err)
	}
	cases := map[string]func(*settings.Instance){
		"empty name":               func(in *settings.Instance) { in.InstanceName = " " },
		"bad base url":             func(in *settings.Instance) { in.BaseURL = "not a url" },
		"bad support email":        func(in *settings.Instance) { in.SupportEmail = "nope" },
		"zero rows":                func(in *settings.Instance) { in.QueryMaxResultRows = 0 },
		"negative background":      func(in *settings.Instance) { in.ExportsBackgroundMaxBytes = -1 },
		"unknown log level":        func(in *settings.Instance) { in.LogLevel = "loud" },
		"smtp without host":        func(in *settings.Instance) { in.SMTPEnabled, in.SMTPFrom = true, "a@b.c" },
		"unknown history mode":     func(in *settings.Instance) { in.QueryHistoryMode = "cloud" },
		"unknown favorites mode":   func(in *settings.Instance) { in.QueryFavoritesMode = "cloud" },
		"retention above max":      func(in *settings.Instance) { in.QueryHistoryRetentionCount = in.QueryHistoryRetentionCountMax + 1 },
		"token ttl out of range":   func(in *settings.Instance) { in.JWTAccessTokenTTLSeconds = settings.MaxDurationSeconds + 1 },
		"worker count above limit": func(in *settings.Instance) { in.JobsWorkerCount = 257 },
	}
	for name, mutate := range cases {
		in := valid
		mutate(&in)
		if err := settings.Validate(in); err == nil {
			t.Fatalf("%s: Validate accepted invalid settings", name)
		}
	}
	for _, mode := range []string{settings.HistoryModeBackend, settings.HistoryModeLocal, settings.HistoryModeOff} {
		if !settings.IsSupportedHistoryMode(mode) {
			t.Fatalf("mode %q unsupported", mode)
		}
	}
	if settings.IsSupportedHistoryMode("") {
		t.Fatal("empty mode supported")
	}
}

func TestUpdateInstanceSealsPasswordAndNotifies(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	svc := newService(store)
	var published []settings.Instance
	svc.Subscribe(func(in settings.Instance) { published = append(published, in) })

	out, err := svc.UpdateInstance(ctx, instanceAdmin(adminID), settings.UpdateInstanceInput{
		InstanceName: ptr("  Renamed  "),
		SMTPPassword: set("secret"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.InstanceName != "Renamed" || out.SMTPPasswordEncrypted != "sealed:secret" {
		t.Fatalf("stored = %q/%q", out.InstanceName, out.SMTPPasswordEncrypted)
	}
	if len(published) != 1 || published[0].InstanceName != "Renamed" {
		t.Fatalf("published = %+v", published)
	}

	out, err = svc.UpdateInstance(ctx, instanceAdmin(adminID), settings.UpdateInstanceInput{SMTPPassword: settings.Patch[string]{Set: true}})
	if err != nil {
		t.Fatal(err)
	}
	if out.SMTPPasswordEncrypted != "" {
		t.Fatal("null password did not clear the stored password")
	}
}

func TestUpdateInstanceRejectsInvalidInput(t *testing.T) {
	ctx := context.Background()
	cases := map[string]settings.UpdateInstanceInput{
		"empty patch":          {},
		"field error":          {QueryMaxResultRows: ptr(0)},
		"retention above max":  {QueryHistoryRetentionCount: ptr(database.DefaultQueryHistoryRetentionCountMax + 1)},
		"unknown history mode": {QueryHistoryMode: ptr("cloud")},
	}
	for name, in := range cases {
		store := newFakeStore()
		_, err := newService(store).UpdateInstance(ctx, instanceAdmin(adminID), in)
		var verr *settings.ValidationError
		if !errors.As(err, &verr) || !errors.Is(err, settings.ErrValidation) {
			t.Fatalf("%s: error = %v, want ValidationError", name, err)
		}
		if store.writes != 0 {
			t.Fatalf("%s: rejected update was stored", name)
		}
	}

	store := newFakeStore()
	store.instance.FileRevisionsEnabled = false
	svc := settings.New(settings.Deps{Store: store, Sealer: fakeSealer{}, FileStorageMode: config.FilesStorageModeFile})
	_, err := svc.UpdateInstance(ctx, instanceAdmin(adminID), settings.UpdateInstanceInput{FileRevisionsEnabled: ptr(true)})
	var verr *settings.ValidationError
	if !errors.As(err, &verr) || verr.Validator.FieldErrors["file_revisions_enabled"] == "" {
		t.Fatalf("file mode revisions error = %v", err)
	}
}

func TestUpdateOrgValidatesAgainstInstance(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	store.instance.QueryHistoryMode = settings.HistoryModeOff
	cases := map[string]settings.UpdateOrgInput{
		"empty patch":          {},
		"widens rows":          {QueryMaxResultRows: set(store.instance.QueryMaxResultRows + 1)},
		"enables history":      {QueryHistoryMode: set(settings.HistoryModeBackend)},
		"retention above inst": {QueryHistoryRetentionCount: set(store.instance.QueryHistoryRetentionCount + 1)},
	}
	for name, in := range cases {
		_, err := newService(store).UpdateOrg(ctx, account(writerID), testOrg, in)
		if !errors.Is(err, settings.ErrValidation) {
			t.Fatalf("%s: error = %v, want ErrValidation", name, err)
		}
	}
	if store.writes != 0 {
		t.Fatalf("rejected updates wrote %d times", store.writes)
	}
}

func TestUpdateOrgStoresOverridesAndReportsRemainingRows(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	store.historyRows = true
	store.favoriteRows = true
	store.orgs[testOrg] = database.OrganizationRuntimeSettings{OrgID: testOrg, QueryMaxResultRows: ptr(50)}
	svc := newService(store)

	out, err := svc.UpdateOrg(ctx, account(writerID), testOrg, settings.UpdateOrgInput{
		QueryMaxResultRows: settings.Patch[int]{Set: true},
		QueryHistoryMode:   set(settings.HistoryModeLocal),
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Overrides.QueryMaxResultRows != nil {
		t.Fatal("null patch did not clear the override")
	}
	if out.Effective.QueryHistoryMode != settings.HistoryModeLocal {
		t.Fatalf("effective history mode = %q", out.Effective.QueryHistoryMode)
	}
	if !out.HistoryRowsRemain || out.FavoritesRowsRemain {
		t.Fatalf("rows remain = %v/%v, want true/false", out.HistoryRowsRemain, out.FavoritesRowsRemain)
	}
}

func TestPurgeOrgClearsOnlyThatOrg(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	svc := newService(store)
	if err := svc.PurgeOrgHistory(ctx, account(writerID), testOrg); err != nil {
		t.Fatal(err)
	}
	if err := svc.PurgeOrgFavorites(ctx, account(writerID), testOrg); err != nil {
		t.Fatal(err)
	}
	if len(store.historyCleared) != 1 || store.historyCleared[0] != testOrg || len(store.favoritesCleared) != 1 || store.favoritesCleared[0] != testOrg {
		t.Fatalf("cleared = %v/%v", store.historyCleared, store.favoritesCleared)
	}
}

func TestRotateSMTPPassword(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	svc := newService(store)
	var notified int
	svc.Subscribe(func(settings.Instance) { notified++ })

	scanned, rotated, err := svc.RotateSMTPPassword(ctx, fakeRotator{stale: true})
	if err != nil || scanned || rotated {
		t.Fatalf("no password: scanned=%v rotated=%v err=%v", scanned, rotated, err)
	}

	store.instance.SMTPPasswordEncrypted = "old:secret"
	scanned, rotated, err = svc.RotateSMTPPassword(ctx, fakeRotator{stale: false})
	if err != nil || !scanned || rotated {
		t.Fatalf("current key: scanned=%v rotated=%v err=%v", scanned, rotated, err)
	}

	scanned, rotated, err = svc.RotateSMTPPassword(ctx, fakeRotator{stale: true})
	if err != nil || !scanned || !rotated {
		t.Fatalf("stale key: scanned=%v rotated=%v err=%v", scanned, rotated, err)
	}
	if store.instance.SMTPPasswordEncrypted != "sealed:secret" || notified != 1 {
		t.Fatalf("stored = %q, notified = %d", store.instance.SMTPPasswordEncrypted, notified)
	}
}

func TestInitializeInstanceBaseURL(t *testing.T) {
	ctx := context.Background()

	store := newFakeStore()
	store.instance.BaseURL = ""
	if err := settings.InitializeInstanceBaseURL(ctx, store, " https://boot.example.com "); err != nil {
		t.Fatal(err)
	}
	if store.seededBaseURL != "https://boot.example.com" {
		t.Fatalf("seeded = %q", store.seededBaseURL)
	}

	store = newFakeStore()
	if err := settings.InitializeInstanceBaseURL(ctx, store, "https://boot.example.com"); err != nil {
		t.Fatal(err)
	}
	if store.seededBaseURL != "" {
		t.Fatal("stored base URL was overwritten")
	}

	store = newFakeStore()
	store.instance.BaseURL = ""
	store.anyAdmin = true
	if err := settings.InitializeInstanceBaseURL(ctx, store, "https://boot.example.com"); err == nil {
		t.Fatal("configured instance without base URL was accepted")
	}
	if store.seededBaseURL != "" {
		t.Fatal("configured instance was seeded")
	}

	store = newFakeStore()
	store.instanceFound = false
	if err := settings.InitializeInstanceBaseURL(ctx, store, "https://boot.example.com"); err == nil {
		t.Fatal("missing row was accepted")
	}
}

func TestValidateInvariant(t *testing.T) {
	ctx := context.Background()
	if err := settings.ValidateInvariant(ctx, newFakeStore()); err != nil {
		t.Fatal(err)
	}
	missing := newFakeStore()
	missing.instanceFound = false
	if err := settings.ValidateInvariant(ctx, missing); err == nil {
		t.Fatal("missing row was accepted")
	}
	invalid := newFakeStore()
	invalid.instance.QueryMaxResultRows = 0
	if err := settings.ValidateInvariant(ctx, invalid); err == nil {
		t.Fatal("invalid row was accepted")
	}
}
