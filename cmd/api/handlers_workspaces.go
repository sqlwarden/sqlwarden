package main

import (
	"net/http"
	"sort"
	"strings"

	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
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

	var (
		result response.Paginated[database.Workspace]
		err    error
	)
	if app.config.desktopMode {
		result, err = app.db.ListWorkspacesPage(r.Context(), database.ListWorkspacesParams{
			OwnerType: "org",
			OwnerID:   org.ID,
			Search:    q.Search,
			Name:      name,
			Sort:      q.Sort,
			Order:     q.Order,
			Page:      q.Page,
			PageSize:  q.PageSize,
		})
	} else {
		account := contextGetAccount(r)
		var workspaces []database.Workspace
		workspaces, err = app.db.ListAccessibleWorkspaces(r.Context(), account.ID, org.ID)
		if err == nil {
			workspaces = filterAccessibleWorkspaces(workspaces, q.Search, name, q.Sort, q.Order)
			result = response.PaginateItems(workspaces, q.Page, q.PageSize)
		}
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

	input.V.CheckField(input.Name != "", "name", "name is required")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	org := contextGetOrg(r)
	account := contextGetAccount(r)
	ws, err := app.db.InsertWorkspace(r.Context(), &org.ID, "org", org.ID, input.Name, input.Description)
	if err != nil {
		if isUniqueViolation(err) {
			app.failedDuplicateField(w, r, "name", "a workspace with this name already exists in this organization")
			return
		}
		app.serverError(w, r, err)
		return
	}

	err = app.enforcer.SeedWorkspace(r.Context(), org.ID, ws.ID, account.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = response.JSON(w, http.StatusCreated, ws)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getWorkspace(w http.ResponseWriter, r *http.Request) {
	ws := contextGetWorkspace(r)
	if ws.OwnerType == "org" && !app.config.desktopMode {
		account := contextGetAccount(r)
		org := contextGetOrg(r)
		workspaces, err := app.db.ListAccessibleWorkspaces(r.Context(), account.ID, org.ID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if !containsWorkspace(workspaces, ws.ID) {
			app.notFound(w, r)
			return
		}
	}
	err := response.JSON(w, http.StatusOK, ws)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func containsWorkspace(workspaces []database.Workspace, workspaceID int64) bool {
	for _, workspace := range workspaces {
		if workspace.ID == workspaceID {
			return true
		}
	}
	return false
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

	input.V.CheckField(input.Name != "", "name", "name is required")
	input.V.CheckField(input.OrgID == nil, "org_id", "is immutable")
	input.V.CheckField(input.OwnerType == nil, "owner_type", "is immutable")
	input.V.CheckField(input.OwnerID == nil, "owner_id", "is immutable")
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
