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

func (app *application) listEnvironments(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)
	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"name":       "name",
		"created_at": "created_at",
	})
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}

	var (
		envs []database.Environment
		err  error
	)
	if app.config.desktopMode {
		envs, err = app.db.ListEnvironmentsFiltered(r.Context(), database.ListEnvironmentsParams{
			WorkspaceID: ws.ID,
			Sort:        q.Sort,
			Order:       q.Order,
		})
	} else {
		account := contextGetAccount(r)
		envs, err = app.db.ListAccessibleEnvironments(r.Context(), account.ID, org.ID, ws.ID)
		if err == nil {
			envs = sortAccessibleEnvironments(envs, q.Sort, q.Order)
		}
	}
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	err = response.JSON(w, http.StatusOK, envs)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func sortAccessibleEnvironments(envs []database.Environment, sortBy, order string) []database.Environment {
	sort.Slice(envs, func(i, j int) bool {
		cmp := compareEnvironment(envs[i], envs[j], sortBy)
		if order == "desc" {
			return cmp > 0
		}
		return cmp < 0
	})
	return envs
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

	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)
	env, err := app.db.InsertEnvironment(r.Context(), ws.ID, &org.ID, ws.OwnerType, ws.OwnerID, input.Name, input.Description)
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
	err := response.JSON(w, http.StatusOK, env)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) updateEnvironment(w http.ResponseWriter, r *http.Request) {
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
		app.serverError(w, r, err)
		return
	}

	app.enforcer.InvalidateAncestry("environment", env.ID)
	for _, cid := range connIDs {
		app.enforcer.InvalidateAncestry("connection", cid)
	}
	w.WriteHeader(http.StatusNoContent)
}
