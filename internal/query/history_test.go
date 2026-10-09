package query

import (
	"context"
	"errors"
	"testing"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/access/accesstest"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/response"
)

const (
	testOrg       int64 = 1
	testWorkspace int64 = 10
	testEnv       int64 = 20
	testConn      int64 = 30
	userA         int64 = 7
	userB         int64 = 8
	orgOnlyUser   int64 = 9
	outsider      int64 = 99

	otherWorkspace     int64 = 11
	multiWorkspaceUser int64 = 12
)

type fakeDirectory struct{}

func (fakeDirectory) GetOrg(_ context.Context, id int64) (access.Org, bool, error) {
	return access.Org{ID: id}, id == testOrg, nil
}

func (fakeDirectory) IsOrgMember(_ context.Context, orgID, accountID int64) (bool, error) {
	return orgID == testOrg && accountID != outsider, nil
}

func (fakeDirectory) IsEffectiveWorkspaceMember(_ context.Context, _, workspaceID, accountID int64) (bool, error) {
	if accountID == multiWorkspaceUser {
		return workspaceID == testWorkspace || workspaceID == otherWorkspace, nil
	}
	return workspaceID == testWorkspace && (accountID == userA || accountID == userB), nil
}

func (fakeDirectory) GetWorkspace(_ context.Context, id int64) (access.Workspace, bool, error) {
	org := testOrg
	switch id {
	case testWorkspace, 11:
		return access.Workspace{ID: id, OrgID: &org, OwnerType: "org", OwnerID: org}, true, nil
	}
	return access.Workspace{}, false, nil
}

func (fakeDirectory) GetConnection(_ context.Context, id int64) (access.Connection, bool, error) {
	if id == testConn {
		return access.Connection{ID: id, WorkspaceID: testWorkspace, EnvironmentID: testEnv}, true, nil
	}
	return access.Connection{}, false, nil
}

type fakeSettings struct {
	policy Policy
	calls  int
}

func (f *fakeSettings) ForWorkspace(context.Context, access.Workspace) (Policy, error) {
	f.calls++
	return f.policy, nil
}

type fakeStore struct {
	calls     int
	history   []database.QueryHistoryEntry
	favorites []database.QueryFavorite
	retention int
	nextID    int64

	workspaceHistoryConnection *int64
}

func (f *fakeStore) InsertQueryHistoryEntry(_ context.Context, e database.QueryHistoryEntry, retention int) (database.QueryHistoryEntry, error) {
	f.calls++
	f.retention = retention
	f.nextID++
	e.ID = f.nextID
	f.history = append(f.history, e)
	return e, nil
}

func (f *fakeStore) ListQueryHistory(_ context.Context, connectionID, accountID int64, _ string, page, pageSize int) (response.Paginated[database.QueryHistoryEntry], error) {
	f.calls++
	var items []database.QueryHistoryEntry
	for _, e := range f.history {
		if e.ConnectionID == connectionID && e.AccountID == accountID {
			items = append(items, e)
		}
	}
	return response.Paginated[database.QueryHistoryEntry]{Items: items, Page: page, PageSize: pageSize, Total: len(items)}, nil
}

func (f *fakeStore) ListQueryHistoryForWorkspace(_ context.Context, _, accountID int64, connectionID *int64, _ string, page, pageSize int) (response.Paginated[database.QueryHistoryEntry], error) {
	f.calls++
	f.workspaceHistoryConnection = connectionID
	var items []database.QueryHistoryEntry
	for _, e := range f.history {
		if e.AccountID == accountID {
			items = append(items, e)
		}
	}
	return response.Paginated[database.QueryHistoryEntry]{Items: items, Page: page, PageSize: pageSize, Total: len(items)}, nil
}

func (f *fakeStore) DeleteQueryHistoryEntry(_ context.Context, id, connectionID, accountID int64) (bool, error) {
	f.calls++
	for i, e := range f.history {
		if e.ID == id && e.ConnectionID == connectionID && e.AccountID == accountID {
			f.history = append(f.history[:i], f.history[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeStore) ClearQueryHistoryForConnection(_ context.Context, connectionID, accountID int64) error {
	f.calls++
	kept := f.history[:0]
	for _, e := range f.history {
		if e.ConnectionID != connectionID || e.AccountID != accountID {
			kept = append(kept, e)
		}
	}
	f.history = kept
	return nil
}

func (f *fakeStore) ListQueryFavorites(_ context.Context, workspaceID, accountID int64, _ string) ([]database.QueryFavorite, error) {
	f.calls++
	var items []database.QueryFavorite
	for _, fav := range f.favorites {
		if fav.WorkspaceID == workspaceID && fav.AccountID == accountID {
			items = append(items, fav)
		}
	}
	return items, nil
}

func (f *fakeStore) CreateQueryFavorite(_ context.Context, fav database.QueryFavorite) (database.QueryFavorite, error) {
	f.calls++
	f.nextID++
	fav.ID = f.nextID
	f.favorites = append(f.favorites, fav)
	return fav, nil
}

func (f *fakeStore) UpdateQueryFavorite(_ context.Context, id, workspaceID, accountID int64, name, sqlText string) (database.QueryFavorite, bool, error) {
	f.calls++
	for i, fav := range f.favorites {
		if fav.ID == id && fav.WorkspaceID == workspaceID && fav.AccountID == accountID {
			f.favorites[i].Name, f.favorites[i].SQL = name, sqlText
			return f.favorites[i], true, nil
		}
	}
	return database.QueryFavorite{}, false, nil
}

func (f *fakeStore) DeleteQueryFavorite(_ context.Context, id, workspaceID, accountID int64) (bool, error) {
	f.calls++
	for i, fav := range f.favorites {
		if fav.ID == id && fav.WorkspaceID == workspaceID && fav.AccountID == accountID {
			f.favorites = append(f.favorites[:i], f.favorites[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

func newTestService(policy Policy) (*Service, *fakeStore, *fakeSettings) {
	store := &fakeStore{}
	settings := &fakeSettings{policy: policy}
	svc := New(Deps{Store: store, Settings: settings, Requirer: access.NewRequirer(fakeDirectory{}, nil)})
	return svc, store, settings
}

func backendPolicy() Policy {
	return Policy{HistoryMode: "backend", HistoryRetention: 42, FavoritesMode: "backend"}
}

func principal(id int64) access.Principal {
	return access.Principal{Subject: access.SubjectRef{Kind: access.SubjectAccount, ID: id}}
}

func connRef() access.ConnRef {
	return access.ConnRef{OrgID: testOrg, WorkspaceID: testWorkspace, ConnectionID: testConn}
}

func validEntry() HistoryInput {
	return HistoryInput{SQL: "SELECT 1", Status: "ok"}
}

func TestHistoryDeniesBeforeStoreAccess(t *testing.T) {
	tests := []struct {
		name string
		p    access.Principal
		ref  access.ConnRef
		want error
	}{
		{"non-member", principal(outsider), connRef(), ErrForbidden},
		{"org member without workspace access", principal(orgOnlyUser), connRef(), ErrForbidden},
		{"unknown connection", principal(userA), access.ConnRef{OrgID: testOrg, WorkspaceID: testWorkspace, ConnectionID: 404}, ErrNotFound},
		{"connection in another workspace", principal(userA), access.ConnRef{OrgID: testOrg, WorkspaceID: 11, ConnectionID: testConn}, ErrNotFound},
		{"unknown workspace", principal(userA), access.ConnRef{OrgID: testOrg, WorkspaceID: 404, ConnectionID: testConn}, ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, store, settings := newTestService(backendPolicy())
			ctx := context.Background()
			_, errRecord := svc.RecordHistory(ctx, tt.p, tt.ref, validEntry())
			_, errList := svc.ListHistory(ctx, tt.p, tt.ref, ListQuery{Page: 1, PageSize: 25})
			errDelete := svc.DeleteHistoryEntry(ctx, tt.p, tt.ref, 1)
			errClear := svc.ClearHistory(ctx, tt.p, tt.ref)
			for op, err := range map[string]error{"record": errRecord, "list": errList, "delete": errDelete, "clear": errClear} {
				accesstest.AssertDenies(t, err, tt.want, op)
			}
			if store.calls != 0 || settings.calls != 0 {
				t.Fatalf("store calls = %d, settings calls = %d, want none on denial", store.calls, settings.calls)
			}
		})
	}
}

func TestListWorkspaceHistoryDenies(t *testing.T) {
	tests := []struct {
		name string
		p    access.Principal
		ws   int64
		want error
	}{
		{"non-member", principal(outsider), testWorkspace, ErrForbidden},
		{"org member without workspace access", principal(orgOnlyUser), testWorkspace, ErrForbidden},
		{"member of a different workspace only", principal(userA), 11, ErrForbidden},
		{"unknown workspace", principal(userA), 404, ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, store, _ := newTestService(backendPolicy())
			_, err := svc.ListWorkspaceHistory(context.Background(), tt.p, testOrg, tt.ws, nil, ListQuery{Page: 1, PageSize: 25})
			accesstest.AssertDenies(t, err, tt.want, "workspace history")
			if store.calls != 0 {
				t.Fatal("store called on denial")
			}
		})
	}
}

func TestListWorkspaceHistoryPassesConnectionFilter(t *testing.T) {
	svc, store, _ := newTestService(backendPolicy())
	conn := testConn
	if _, err := svc.ListWorkspaceHistory(context.Background(), principal(userA), testOrg, testWorkspace, &conn, ListQuery{Page: 1, PageSize: 25}); err != nil {
		t.Fatal(err)
	}
	if store.workspaceHistoryConnection == nil || *store.workspaceHistoryConnection != testConn {
		t.Fatalf("store connection filter = %v, want %d", store.workspaceHistoryConnection, testConn)
	}
}

func TestRecordHistoryValidation(t *testing.T) {
	svc, store, _ := newTestService(backendPolicy())
	_, err := svc.RecordHistory(context.Background(), principal(userA), connRef(), HistoryInput{Status: "weird"})
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("error = %v, want ValidationError", err)
	}
	if verr.Fields["sql_text"] != "SQL text is required." || verr.Fields["status"] != "Status must be ok, error, or cancelled." {
		t.Fatalf("fields = %v", verr.Fields)
	}
	if store.calls != 0 {
		t.Fatalf("store called %d times on invalid input", store.calls)
	}
}

func TestRecordHistoryDisabled(t *testing.T) {
	for _, mode := range []string{"off", "local", ""} {
		svc, store, _ := newTestService(Policy{HistoryMode: mode, HistoryRetention: 5})
		_, err := svc.RecordHistory(context.Background(), principal(userA), connRef(), validEntry())
		var disabled *DisabledError
		if !errors.Is(err, ErrDisabled) || !errors.As(err, &disabled) || disabled.Message != "Query history is not enabled for this connection." {
			t.Fatalf("mode %q error = %v", mode, err)
		}
		if store.calls != 0 {
			t.Fatalf("mode %q store called", mode)
		}
	}
}

func TestRecordHistoryPassesRetentionAndOwner(t *testing.T) {
	svc, store, _ := newTestService(backendPolicy())
	msg := "boom"
	saved, err := svc.RecordHistory(context.Background(), principal(userA), connRef(), HistoryInput{SQL: "SELECT 1", Status: "error", ErrorMessage: &msg, DurationMS: 5, RowsAffected: 2})
	if err != nil {
		t.Fatal(err)
	}
	if store.retention != 42 {
		t.Fatalf("retention = %d, want 42", store.retention)
	}
	if saved.AccountID != userA || saved.ConnectionID != testConn || saved.DurationMS != 5 || saved.RowsAffected != 2 || saved.ErrorMessage != &msg {
		t.Fatalf("saved = %+v", saved)
	}
}

func TestHistoryIsOwnerScoped(t *testing.T) {
	svc, _, _ := newTestService(backendPolicy())
	ctx := context.Background()
	entry, err := svc.RecordHistory(ctx, principal(userA), connRef(), validEntry())
	if err != nil {
		t.Fatal(err)
	}

	accesstest.AssertNotFound(t, svc.DeleteHistoryEntry(ctx, principal(userB), connRef(), entry.ID), "user B delete")
	listed, err := svc.ListHistory(ctx, principal(userB), connRef(), ListQuery{Page: 1, PageSize: 25})
	if err != nil || len(listed.Items) != 0 {
		t.Fatalf("user B list = %+v, %v", listed, err)
	}
	if err := svc.ClearHistory(ctx, principal(userB), connRef()); err != nil {
		t.Fatal(err)
	}
	listed, err = svc.ListWorkspaceHistory(ctx, principal(userA), testOrg, testWorkspace, nil, ListQuery{Page: 1, PageSize: 25})
	if err != nil || len(listed.Items) != 1 {
		t.Fatalf("user A workspace list = %+v, %v", listed, err)
	}
	if err := svc.DeleteHistoryEntry(ctx, principal(userA), connRef(), entry.ID); err != nil {
		t.Fatalf("owner delete error = %v", err)
	}
}
