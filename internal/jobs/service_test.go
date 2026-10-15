package jobs

import (
	"context"
	"errors"
	"testing"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/access/accesstest"
	"github.com/sqlwarden/internal/response"
)

const (
	serviceOrg       int64 = 1
	serviceWorkspace int64 = 10
	serviceAccount   int64 = 7
)

type serviceDirectory struct {
	members map[[3]int64]bool
}

func (d serviceDirectory) GetOrg(_ context.Context, id int64) (access.Org, bool, error) {
	return access.Org{ID: id}, id == serviceOrg, nil
}

func (d serviceDirectory) IsOrgMember(_ context.Context, orgID, accountID int64) (bool, error) {
	return orgID == serviceOrg && (accountID == serviceAccount || accountID == 8), nil
}

func (d serviceDirectory) IsEffectiveWorkspaceMember(_ context.Context, orgID, workspaceID, accountID int64) (bool, error) {
	return d.members[[3]int64{orgID, workspaceID, accountID}], nil
}

func (serviceDirectory) GetWorkspace(_ context.Context, id int64) (access.Workspace, bool, error) {
	orgID := serviceOrg
	switch id {
	case serviceWorkspace:
		return access.Workspace{ID: id, OrgID: &orgID, OwnerType: "org", OwnerID: orgID}, true, nil
	case 20:
		foreignOrgID := int64(2)
		return access.Workspace{ID: id, OrgID: &foreignOrgID, OwnerType: "org", OwnerID: foreignOrgID}, true, nil
	default:
		return access.Workspace{}, false, nil
	}
}

func (serviceDirectory) GetConnection(context.Context, int64) (access.Connection, bool, error) {
	return access.Connection{}, false, nil
}

type storeCall struct {
	orgID, workspaceID, accountID int64
	page, pageSize                int
	jobID, afterID                string
	limit                         int
}

type serviceStore struct {
	calls       int
	listCall    storeCall
	getCall     storeCall
	eventsCall  storeCall
	cancelCall  storeCall
	getRecord   Record
	getFound    bool
	cancelJob   Record
	cancelFound bool
}

func (s *serviceStore) ListUserWorkspaceJobs(_ context.Context, orgID, workspaceID, accountID int64, page, pageSize int) (response.Paginated[Record], error) {
	s.calls++
	s.listCall = storeCall{orgID: orgID, workspaceID: workspaceID, accountID: accountID, page: page, pageSize: pageSize}
	return response.Paginated[Record]{Page: page, PageSize: pageSize}, nil
}

func (s *serviceStore) GetUserWorkspaceJob(_ context.Context, orgID, workspaceID, accountID int64, jobID string) (Record, bool, error) {
	s.calls++
	s.getCall = storeCall{orgID: orgID, workspaceID: workspaceID, accountID: accountID, jobID: jobID}
	return s.getRecord, s.getFound, nil
}

func (s *serviceStore) ListUserWorkspaceJobEvents(_ context.Context, orgID, workspaceID, accountID int64, jobID, afterID string, limit int) (EventPage, error) {
	s.calls++
	s.eventsCall = storeCall{orgID: orgID, workspaceID: workspaceID, accountID: accountID, jobID: jobID, afterID: afterID, limit: limit}
	return EventPage{}, nil
}

func (s *serviceStore) RequestCancelUserWorkspaceJob(_ context.Context, orgID, workspaceID, accountID int64, jobID string) (Record, bool, error) {
	s.calls++
	s.cancelCall = storeCall{orgID: orgID, workspaceID: workspaceID, accountID: accountID, jobID: jobID}
	return s.cancelJob, s.cancelFound, nil
}

func newServiceTest(store Store) *Service {
	dir := serviceDirectory{members: map[[3]int64]bool{
		{serviceOrg, serviceWorkspace, serviceAccount}: true,
	}}
	return NewService(store, access.NewRequirer(dir, nil))
}

func servicePrincipal(accountID int64) access.Principal {
	return access.Principal{Subject: access.SubjectRef{Kind: access.SubjectAccount, ID: accountID}}
}

func TestServiceRejectsNonMemberBeforeStore(t *testing.T) {
	store := &serviceStore{}
	_, err := newServiceTest(store).List(context.Background(), servicePrincipal(8), serviceOrg, serviceWorkspace, "2", "25")
	accesstest.AssertForbidden(t, err, "List")
	if store.calls != 0 {
		t.Fatalf("store calls = %d, want 0", store.calls)
	}
}

func TestServiceAuthorizesBeforeValidatingPaging(t *testing.T) {
	store := &serviceStore{}
	svc := newServiceTest(store)
	ctx := context.Background()

	_, err := svc.List(ctx, servicePrincipal(8), serviceOrg, serviceWorkspace, "0", "x")
	accesstest.AssertForbidden(t, err, "List")
	_, err = svc.Events(ctx, servicePrincipal(8), serviceOrg, serviceWorkspace, "job-1", "", "0")
	accesstest.AssertForbidden(t, err, "Events")
	if store.calls != 0 {
		t.Fatalf("store calls = %d, want 0", store.calls)
	}
}

func TestServiceRejectsInvalidPagingForMembers(t *testing.T) {
	store := &serviceStore{}
	svc := newServiceTest(store)
	ctx := context.Background()
	p := servicePrincipal(serviceAccount)

	cases := []struct{ page, pageSize, field string }{
		{"0", "", "page"},
		{"abc", "", "page"},
		{"", "0", "page_size"},
		{"", "101", "page_size"},
	}
	for _, tc := range cases {
		_, err := svc.List(ctx, p, serviceOrg, serviceWorkspace, tc.page, tc.pageSize)
		var validation *ValidationError
		if !errors.As(err, &validation) || validation.Fields[tc.field] == "" {
			t.Fatalf("List(%q, %q) error = %v, want validation error on %s", tc.page, tc.pageSize, err, tc.field)
		}
	}
	_, err := svc.Events(ctx, p, serviceOrg, serviceWorkspace, "job-1", "", "0")
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Fields["page_size"] == "" {
		t.Fatalf("Events error = %v, want validation error on page_size", err)
	}
	if store.calls != 0 {
		t.Fatalf("store calls = %d, want 0", store.calls)
	}
}

func TestServiceGetHidesAnotherAccountsJob(t *testing.T) {
	store := &serviceStore{getFound: false}
	_, err := newServiceTest(store).Get(context.Background(), servicePrincipal(serviceAccount), serviceOrg, serviceWorkspace, "other-job")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get error = %v, want %v", err, ErrNotFound)
	}
	want := storeCall{orgID: serviceOrg, workspaceID: serviceWorkspace, accountID: serviceAccount, jobID: "other-job"}
	if store.getCall != want {
		t.Fatalf("Get call = %+v, want %+v", store.getCall, want)
	}
}

func TestServiceRejectsForeignWorkspaceBeforeStore(t *testing.T) {
	store := &serviceStore{}
	_, err := newServiceTest(store).Get(context.Background(), servicePrincipal(serviceAccount), serviceOrg, 20, "foreign-workspace-job")
	accesstest.AssertNotFound(t, err, "Get")
	if store.calls != 0 {
		t.Fatalf("store calls = %d, want 0", store.calls)
	}
}

func TestServicePassesAccountAndPaginationToStore(t *testing.T) {
	store := &serviceStore{cancelJob: Record{ID: "job-1"}, cancelFound: true}
	svc := newServiceTest(store)
	p := servicePrincipal(serviceAccount)
	ctx := context.Background()

	if _, err := svc.List(ctx, p, serviceOrg, serviceWorkspace, "3", "40"); err != nil {
		t.Fatal(err)
	}
	if want := (storeCall{orgID: serviceOrg, workspaceID: serviceWorkspace, accountID: serviceAccount, page: 3, pageSize: 40}); store.listCall != want {
		t.Fatalf("List call = %+v, want %+v", store.listCall, want)
	}

	if _, err := svc.Events(ctx, p, serviceOrg, serviceWorkspace, "job-1", "event-9", "75"); err != nil {
		t.Fatal(err)
	}
	if want := (storeCall{orgID: serviceOrg, workspaceID: serviceWorkspace, accountID: serviceAccount, jobID: "job-1", afterID: "event-9", limit: 75}); store.eventsCall != want {
		t.Fatalf("Events call = %+v, want %+v", store.eventsCall, want)
	}

	if _, err := svc.RequestCancel(ctx, p, serviceOrg, serviceWorkspace, "job-1"); err != nil {
		t.Fatal(err)
	}
	if want := (storeCall{orgID: serviceOrg, workspaceID: serviceWorkspace, accountID: serviceAccount, jobID: "job-1"}); store.cancelCall != want {
		t.Fatalf("RequestCancel call = %+v, want %+v", store.cancelCall, want)
	}
}
