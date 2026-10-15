package web

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/database"
)

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

func TestServicesWiringDirectoryAdapterForwards(t *testing.T) {
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

func TestServicesWiringDirectoryAdapterRejectsNilStore(t *testing.T) {
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

func TestServicesWiringRequirerIsLazySingleton(t *testing.T) {
	app := new(application)

	first := app.requirer()
	second := app.requirer()

	assert.NotNil(t, first)
	assert.Equal(t, second, first)
}

func TestServiceError(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantStatus  int
		wantCode    string
		wantMessage string
	}{
		{
			name:        "forbidden",
			err:         errors.Join(errors.New("authorize request"), access.ErrForbidden),
			wantStatus:  http.StatusForbidden,
			wantCode:    apiErrorNotPermitted,
			wantMessage: "You do not have permission to perform this action.",
		},
		{
			name:        "not found",
			err:         errors.Join(errors.New("load resource"), access.ErrNotFound),
			wantStatus:  http.StatusNotFound,
			wantCode:    apiErrorNotFound,
			wantMessage: "The requested resource could not be found.",
		},
		{
			name:        "unknown",
			err:         errors.New("unexpected failure"),
			wantStatus:  http.StatusInternalServerError,
			wantCode:    apiErrorInternalServer,
			wantMessage: "The server encountered a problem and could not process your request.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newTestApplication(t)
			req := newTestRequest(t, http.MethodGet, "/test", nil)

			res := send(t, req, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				app.serviceError(w, r, tt.err)
			}))

			assert.Equal(t, res.StatusCode, tt.wantStatus)
			assertAPIError(t, res, tt.wantCode, tt.wantMessage)
		})
	}
}
