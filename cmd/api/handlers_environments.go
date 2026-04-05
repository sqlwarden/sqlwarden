package main

import (
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

func (app *application) listEnvironments(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)
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
		result response.Paginated[database.Environment]
		err    error
	)
	if app.config.desktopMode {
		result, err = app.db.ListEnvironmentsPage(r.Context(), database.ListEnvironmentsParams{
			WorkspaceID: ws.ID,
			Search:      q.Search,
			Name:        name,
			Sort:        q.Sort,
			Order:       q.Order,
			Page:        q.Page,
			PageSize:    q.PageSize,
		})
	} else {
		account := contextGetAccount(r)
		var envs []database.Environment
		envs, err = app.db.ListAccessibleEnvironments(r.Context(), account.ID, org.ID, ws.ID)
		if err == nil {
			envs = filterAndSortAccessibleEnvironments(envs, q.Search, name, q.Sort, q.Order)
			result = response.PaginateItems(envs, q.Page, q.PageSize)
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

func filterAndSortAccessibleEnvironments(envs []database.Environment, search, name, sortBy, order string) []database.Environment {
	filtered := make([]database.Environment, 0, len(envs))
	search = strings.ToLower(strings.TrimSpace(search))
	name = strings.TrimSpace(name)

	for _, env := range envs {
		if search != "" && !strings.Contains(strings.ToLower(env.Name), search) {
			continue
		}
		if name != "" && env.Name != name {
			continue
		}
		filtered = append(filtered, env)
	}

	sort.Slice(filtered, func(i, j int) bool {
		cmp := compareEnvironment(filtered[i], filtered[j], sortBy)
		if order == "desc" {
			return cmp > 0
		}
		return cmp < 0
	})
	return filtered
}

func compareEnvironment(left, right database.Environment, sortBy string) int {
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

func (app *application) createEnvironment(w http.ResponseWriter, r *http.Request) {
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

	ws := contextGetWorkspace(r)
	env, err := app.db.InsertEnvironment(r.Context(), ws.ID, input.Name, input.Description)
	if err != nil {
		if isUniqueViolation(err) {
			app.failedDuplicateField(w, r, "name", "an environment with this name already exists in this workspace")
			return
		}
		app.serverError(w, r, err)
		return
	}

	err = response.JSON(w, http.StatusCreated, env)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getEnvironment(w http.ResponseWriter, r *http.Request) {
	env := contextGetEnvironment(r)
	ws := contextGetWorkspace(r)
	if ws.OwnerType == "org" && !app.config.desktopMode {
		account := contextGetAccount(r)
		org := contextGetOrg(r)
		ok, err := app.db.HasAccessibleEnvironment(r.Context(), account.ID, org.ID, ws.ID, env.ID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if !ok {
			app.notFound(w, r)
			return
		}
	}
	err := response.JSON(w, http.StatusOK, env)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) updateEnvironment(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name        string              `json:"name"`
		Description string              `json:"description"`
		WorkspaceID *int64              `json:"workspace_id"`
		V           validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.V.CheckField(input.Name != "", "name", "name is required")
	input.V.CheckField(input.WorkspaceID == nil, "workspace_id", "is immutable")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	env := contextGetEnvironment(r)
	err = app.db.UpdateEnvironment(r.Context(), env.ID, input.Name, input.Description)
	if err != nil {
		if isUniqueViolation(err) {
			app.failedDuplicateField(w, r, "name", "an environment with this name already exists in this workspace")
			return
		}
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (app *application) deleteEnvironment(w http.ResponseWriter, r *http.Request) {
	env := contextGetEnvironment(r)

	// Collect connections tagged to this environment before deletion so we can
	// invalidate their ancestry caches (connection → environment rows will be removed).
	connIDs, err := app.db.ListConnectionIDsByEnvironment(r.Context(), env.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = app.db.DeleteEnvironment(r.Context(), env.ID)
	if err != nil {
		if errors.Is(err, database.ErrEnvironmentHasConnections) {
			app.errorMessage(w, r, http.StatusUnprocessableEntity, err.Error(), nil)
			return
		}
		app.serverError(w, r, err)
		return
	}

	app.enforcer.InvalidateAncestry("environment", env.ID)
	for _, cid := range connIDs {
		app.enforcer.InvalidateAncestry("connection", cid)
	}
	w.WriteHeader(http.StatusNoContent)
}
