package catalog

import (
	"context"
	"errors"
	"testing"

	"github.com/sqlwarden/internal/database"
)

func TestCreateEnvironmentValidatesAndMapsDuplicate(t *testing.T) {
	f := workspaceFixture()
	ctx := context.Background()
	var validation *ValidationError

	_, err := f.service.CreateEnvironment(ctx, f.principal, 1, 10, EnvironmentInput{})
	if !errors.As(err, &validation) || validation.Fields["name"] != "Name is required." {
		t.Fatalf("error = %v, want name validation", err)
	}

	env, err := f.service.CreateEnvironment(ctx, f.principal, 1, 10, EnvironmentInput{Name: "Staging"})
	if err != nil || env.WorkspaceID != 10 {
		t.Fatalf("CreateEnvironment = %+v, %v", env, err)
	}

	f.workspaces.insertEnvErr = errors.New("UNIQUE constraint failed: environments.name")
	_, err = f.service.CreateEnvironment(ctx, f.principal, 1, 10, EnvironmentInput{Name: "Staging"})
	if !errors.As(err, &validation) || validation.Fields["name"] != "An environment with this name already exists in this workspace." {
		t.Fatalf("error = %v, want duplicate name", err)
	}
}

func TestUpdateEnvironmentRejectsWorkspaceChange(t *testing.T) {
	f := workspaceFixture()
	other := int64(11)
	err := f.service.UpdateEnvironment(context.Background(), f.principal, 1, 10, 20, UpdateEnvironmentInput{Name: "x", WorkspaceID: &other})
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Fields["workspace_id"] != "Workspace is immutable." {
		t.Fatalf("error = %v, want workspace validation", err)
	}
	if f.store.environments[20].Name == "x" {
		t.Fatal("invalid update was written")
	}
}

func TestDeleteEnvironmentInvalidatesEnvironmentAndConnectionAncestry(t *testing.T) {
	f := workspaceFixture()
	f.workspaces.envConnections[20] = []int64{30, 31}
	affected, err := f.service.DeleteEnvironment(context.Background(), f.principal, 1, 10, 20)
	if err != nil {
		t.Fatal(err)
	}
	if affected != 2 || len(f.ancestry.invalidated) != 3 {
		t.Fatalf("affected=%d invalidated=%v", affected, f.ancestry.invalidated)
	}
}

func TestDeleteEnvironmentWithConnectionsIsRefused(t *testing.T) {
	f := workspaceFixture()
	f.workspaces.deleteEnvErr = database.ErrEnvironmentHasConnections
	_, err := f.service.DeleteEnvironment(context.Background(), f.principal, 1, 10, 20)
	if !errors.Is(err, ErrEnvironmentHasConnections) {
		t.Fatalf("error = %v, want ErrEnvironmentHasConnections", err)
	}
	if len(f.ancestry.invalidated) != 0 {
		t.Fatal("refused delete invalidated ancestry")
	}
}

func TestListEnvironmentsExcludesInaccessible(t *testing.T) {
	f := workspaceFixture()
	f.store.environments[22] = database.Environment{ID: 22, WorkspaceID: 10, Name: "Hidden"}
	page, err := f.service.ListEnvironments(context.Background(), f.principal, 1, 10, ListQuery{Page: 1, PageSize: 25})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || page.Items[0].ID != 20 {
		t.Fatalf("page = %+v", page)
	}
}
