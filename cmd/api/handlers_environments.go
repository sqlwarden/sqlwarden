package main

import (
	"net/http"

	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

func (app *application) listEnvironments(w http.ResponseWriter, r *http.Request) {
	ws := contextGetWorkspace(r)
	envs, err := app.db.ListEnvironments(r.Context(), ws.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	err = response.JSON(w, http.StatusOK, envs)
	if err != nil {
		app.serverError(w, r, err)
	}
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

