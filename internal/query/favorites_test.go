package query

import (
	"context"
	"errors"
	"testing"

	"github.com/sqlwarden/internal/access/accesstest"
)

func TestFavoritesDenyBeforeStoreAccess(t *testing.T) {
	tests := []struct {
		name string
		p    int64
		ws   int64
		want error
	}{
		{"non-member", outsider, testWorkspace, ErrForbidden},
		{"org member without workspace access", orgOnlyUser, testWorkspace, ErrForbidden},
		{"unknown workspace", userA, 404, ErrNotFound},
	}
	in := FavoriteInput{Name: "n", SQL: "SELECT 1"}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, store, settings := newTestService(backendPolicy())
			ctx := context.Background()
			p := principal(tt.p)
			_, errList := svc.ListFavorites(ctx, p, testOrg, tt.ws, "")
			_, errCreate := svc.CreateFavorite(ctx, p, testOrg, tt.ws, in)
			_, errUpdate := svc.UpdateFavorite(ctx, p, testOrg, tt.ws, 1, in)
			errDelete := svc.DeleteFavorite(ctx, p, testOrg, tt.ws, 1)
			for op, err := range map[string]error{"list": errList, "create": errCreate, "update": errUpdate, "delete": errDelete} {
				accesstest.AssertDenies(t, err, tt.want, op)
			}
			if store.calls != 0 || settings.calls != 0 {
				t.Fatalf("store calls = %d, settings calls = %d, want none on denial", store.calls, settings.calls)
			}
		})
	}
}

func TestFavoriteValidation(t *testing.T) {
	svc, store, _ := newTestService(backendPolicy())
	ctx := context.Background()
	check := func(err error) {
		t.Helper()
		var verr *ValidationError
		if !errors.As(err, &verr) || verr.Fields["name"] != "Name is required." || verr.Fields["sql_text"] != "SQL text is required." {
			t.Fatalf("error = %v", err)
		}
	}
	_, err := svc.CreateFavorite(ctx, principal(userA), testOrg, testWorkspace, FavoriteInput{})
	check(err)
	_, err = svc.UpdateFavorite(ctx, principal(userA), testOrg, testWorkspace, 1, FavoriteInput{})
	check(err)
	if store.calls != 0 {
		t.Fatalf("store called %d times on invalid input", store.calls)
	}
}

func TestCreateFavoriteDisabled(t *testing.T) {
	svc, store, _ := newTestService(Policy{FavoritesMode: "local"})
	_, err := svc.CreateFavorite(context.Background(), principal(userA), testOrg, testWorkspace, FavoriteInput{Name: "n", SQL: "SELECT 1"})
	var disabled *DisabledError
	if !errors.Is(err, ErrDisabled) || !errors.As(err, &disabled) || disabled.Message != "Query favorites are not enabled for this workspace." {
		t.Fatalf("error = %v", err)
	}
	if store.calls != 0 {
		t.Fatal("store called while disabled")
	}
}

func TestFavoriteCannotBeReachedThroughAnotherWorkspace(t *testing.T) {
	svc, store, _ := newTestService(backendPolicy())
	ctx := context.Background()
	p := principal(multiWorkspaceUser)
	fav, err := svc.CreateFavorite(ctx, p, testOrg, testWorkspace, FavoriteInput{Name: "n", SQL: "SELECT 1"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.UpdateFavorite(ctx, p, testOrg, otherWorkspace, fav.ID, FavoriteInput{Name: "x", SQL: "SELECT 2"})
	accesstest.AssertNotFound(t, err, "update through other workspace")
	accesstest.AssertNotFound(t, svc.DeleteFavorite(ctx, p, testOrg, otherWorkspace, fav.ID), "delete through other workspace")
	if len(store.favorites) != 1 || store.favorites[0].Name != "n" {
		t.Fatalf("favorites = %+v, want the original untouched", store.favorites)
	}
}

func TestFavoriteConnectionMustBelongToWorkspace(t *testing.T) {
	svc, store, _ := newTestService(backendPolicy())
	ctx := context.Background()
	p := principal(multiWorkspaceUser)
	foreign := testConn
	unknown := int64(404)

	_, err := svc.CreateFavorite(ctx, p, testOrg, otherWorkspace, FavoriteInput{Name: "n", SQL: "SELECT 1", ConnectionID: &foreign})
	assertConnectionFieldError(t, err, "create foreign workspace")
	_, err = svc.UpdateFavorite(ctx, p, testOrg, otherWorkspace, 1, FavoriteInput{Name: "n", SQL: "SELECT 1", ConnectionID: &foreign})
	assertConnectionFieldError(t, err, "update foreign workspace")
	_, err = svc.CreateFavorite(ctx, p, testOrg, testWorkspace, FavoriteInput{Name: "n", SQL: "SELECT 1", ConnectionID: &unknown})
	assertConnectionFieldError(t, err, "create unknown connection")
	if len(store.favorites) != 0 {
		t.Fatalf("favorites = %+v, want none stored", store.favorites)
	}

	valid := testConn
	fav, err := svc.CreateFavorite(ctx, p, testOrg, testWorkspace, FavoriteInput{Name: "n", SQL: "SELECT 1", ConnectionID: &valid})
	if err != nil || fav.ConnectionID == nil || *fav.ConnectionID != testConn {
		t.Fatalf("create with workspace connection = %+v, %v", fav, err)
	}
	if _, err := svc.UpdateFavorite(ctx, p, testOrg, testWorkspace, fav.ID, FavoriteInput{Name: "m", SQL: "SELECT 2", ConnectionID: &valid}); err != nil {
		t.Fatalf("update with workspace connection: %v", err)
	}
}

func assertConnectionFieldError(t *testing.T, err error, label string) {
	t.Helper()
	var verr *ValidationError
	if !errors.As(err, &verr) || verr.Fields["connection_id"] != "Connection was not found in this workspace." {
		t.Fatalf("%s error = %v", label, err)
	}
}

func TestFavoritesAreOwnerScoped(t *testing.T) {
	svc, _, _ := newTestService(backendPolicy())
	ctx := context.Background()
	connID := testConn
	fav, err := svc.CreateFavorite(ctx, principal(userA), testOrg, testWorkspace, FavoriteInput{Name: "n", SQL: "SELECT 1", ConnectionID: &connID})
	if err != nil {
		t.Fatal(err)
	}
	if fav.AccountID != userA || fav.WorkspaceID != testWorkspace || fav.ConnectionID == nil || *fav.ConnectionID != testConn {
		t.Fatalf("favorite = %+v", fav)
	}

	update := FavoriteInput{Name: "x", SQL: "SELECT 2"}
	_, err = svc.UpdateFavorite(ctx, principal(userB), testOrg, testWorkspace, fav.ID, update)
	accesstest.AssertNotFound(t, err, "user B update")
	accesstest.AssertNotFound(t, svc.DeleteFavorite(ctx, principal(userB), testOrg, testWorkspace, fav.ID), "user B delete")
	list, err := svc.ListFavorites(ctx, principal(userB), testOrg, testWorkspace, "")
	if err != nil || len(list) != 0 {
		t.Fatalf("user B list = %v, %v", list, err)
	}
	updated, err := svc.UpdateFavorite(ctx, principal(userA), testOrg, testWorkspace, fav.ID, update)
	if err != nil || updated.Name != "x" {
		t.Fatalf("owner update = %+v, %v", updated, err)
	}
	if err := svc.DeleteFavorite(ctx, principal(userA), testOrg, testWorkspace, fav.ID); err != nil {
		t.Fatal(err)
	}
}
