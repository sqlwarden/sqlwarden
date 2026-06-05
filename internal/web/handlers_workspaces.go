package web

import (
	"context"
	"net/http"
	"sort"
	"strings"

	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
	"github.com/uptrace/bun"
)

func (app *application) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"name":       "name",
		"created_at": "created_at",
	})
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}

	account := contextGetAccount(r)
	workspaces, err := app.db.ListAccessibleWorkspaces(r.Context(), account.ID, org.ID)
	var result response.Paginated[database.Workspace]
	if err == nil {
		workspaces = filterAccessibleWorkspaces(workspaces, q.Search, name, q.Sort, q.Order)
		result = response.PaginateItems(workspaces, q.Page, q.PageSize)
		err = app.db.PopulateWorkspaceCounts(r.Context(), result.Items)
	}
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = response.JSON(w, http.StatusOK, result)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func filterAccessibleWorkspaces(workspaces []database.Workspace, search, name, sortBy, order string) []database.Workspace {
	filtered := make([]database.Workspace, 0, len(workspaces))
	search = strings.ToLower(strings.TrimSpace(search))
	name = strings.TrimSpace(name)

	for _, workspace := range workspaces {
		if search != "" && !strings.Contains(strings.ToLower(workspace.Name), search) {
			continue
		}
		if name != "" && workspace.Name != name {
			continue
		}
		filtered = append(filtered, workspace)
	}

	sort.Slice(filtered, func(i, j int) bool {
		cmp := compareWorkspace(filtered[i], filtered[j], sortBy)
		if order == "desc" {
			return cmp > 0
		}
		return cmp < 0
	})
	return filtered
}

func compareWorkspace(left, right database.Workspace, sortBy string) int {
	switch sortBy {
	case "name":
		if left.Name != right.Name {
			return strings.Compare(left.Name, right.Name)
		}
	default:
		if !left.CreatedAt.Equal(right.CreatedAt) {
			if left.CreatedAt.Before(right.CreatedAt) {
				return -1
			}
			return 1
		}
	}
	if left.ID < right.ID {
		return -1
	}
	if left.ID > right.ID {
		return 1
	}
	return 0
}

func (app *application) createWorkspace(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name        string              `json:"name"`
		Description string              `json:"description"`
		V           validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.V.CheckField(input.Name != "", "name", "Name is required.")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	org := contextGetOrg(r)
	account := contextGetAccount(r)
	ws, err := app.createOwnedWorkspace(r.Context(), org.ID, account.ID, input.Name, input.Description)
	if err != nil {
		if isUniqueViolation(err) {
			app.failedDuplicateField(w, r, "name", "A workspace with this name already exists in this organization.")
			return
		}
		app.serverError(w, r, err)
		return
	}

	workspaces := []database.Workspace{ws}
	if err := app.db.PopulateWorkspaceCounts(r.Context(), workspaces); err != nil {
		app.serverError(w, r, err)
		return
	}
	ws = workspaces[0]

	err = response.JSON(w, http.StatusCreated, ws)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) createOwnedWorkspace(ctx context.Context, orgID, creatorAccountID int64, name, description string) (database.Workspace, error) {
	var ws database.Workspace
	err := app.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var err error
		ws, err = app.db.InsertWorkspaceWithExecutor(ctx, tx, &orgID, "org", orgID, name, description)
		if err != nil {
			return err
		}
		return app.enforcer.SeedWorkspaceWithExecutor(ctx, tx, orgID, ws.ID, creatorAccountID)
	})
	if err != nil {
		return database.Workspace{}, err
	}

	app.enforcer.InvalidateOrgPolicy(orgID)
	app.enforcer.InvalidatePrincipals(orgID, creatorAccountID)
	return ws, nil
}

func (app *application) getWorkspace(w http.ResponseWriter, r *http.Request) {
	ws := contextGetWorkspace(r)
	if ws.OwnerType == "org" {
		account := contextGetAccount(r)
		org := contextGetOrg(r)
		ok, err := app.db.HasAccessibleWorkspace(r.Context(), account.ID, org.ID, ws.ID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if !ok {
			app.notFound(w, r)
			return
		}
	}
	workspaces := []database.Workspace{ws}
	if err := app.db.PopulateWorkspaceCounts(r.Context(), workspaces); err != nil {
		app.serverError(w, r, err)
		return
	}
	ws = workspaces[0]

	err := response.JSON(w, http.StatusOK, ws)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) updateWorkspace(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name        string              `json:"name"`
		Description string              `json:"description"`
		OrgID       *int64              `json:"org_id"`
		OwnerType   *string             `json:"owner_type"`
		OwnerID     *int64              `json:"owner_id"`
		V           validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.V.CheckField(input.Name != "", "name", "Name is required.")
	input.V.CheckField(input.OrgID == nil, "org_id", "Organization is immutable.")
	input.V.CheckField(input.OwnerType == nil, "owner_type", "Owner type is immutable.")
	input.V.CheckField(input.OwnerID == nil, "owner_id", "Owner is immutable.")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	ws := contextGetWorkspace(r)
	err = app.db.UpdateWorkspace(r.Context(), ws.ID, input.Name, input.Description)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (app *application) deleteWorkspace(w http.ResponseWriter, r *http.Request) {
	ws := contextGetWorkspace(r)
	err := app.db.DeleteWorkspace(r.Context(), ws.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.enforcer.InvalidateAncestry("workspace", ws.ID)
	w.WriteHeader(http.StatusNoContent)
}
