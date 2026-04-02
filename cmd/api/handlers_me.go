package main

import (
	"context"
	"net/http"

	"github.com/sqlwarden/internal/encrypt"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

// listMyWorkspaces returns all personal-space workspaces owned by the authenticated account.
func (app *application) listMyWorkspaces(w http.ResponseWriter, r *http.Request) {
	account := contextGetAccount(r)
	wss, err := app.db.ListWorkspacesByOwner(r.Context(), "space", account.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	err = response.JSON(w, http.StatusOK, wss)
	if err != nil {
		app.serverError(w, r, err)
	}
}

// createMyWorkspace creates a new personal-space workspace for the authenticated account.
func (app *application) createMyWorkspace(w http.ResponseWriter, r *http.Request) {
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

	account := contextGetAccount(r)
	ws, err := app.db.InsertWorkspace(r.Context(), nil, "space", account.ID, input.Name, input.Description)
	if err != nil {
		if isUniqueViolation(err) {
			input.V.AddFieldError("name", "a workspace with this name already exists")
			app.failedValidation(w, r, input.V)
			return
		}
		app.serverError(w, r, err)
		return
	}

	err = response.JSON(w, http.StatusCreated, ws)
	if err != nil {
		app.serverError(w, r, err)
	}
}

// createMyEnvironment creates an environment within a personal-space workspace.
// Passes nil orgID (no org for personal spaces).
func (app *application) createMyEnvironment(w http.ResponseWriter, r *http.Request) {
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
	env, err := app.db.InsertEnvironment(r.Context(), ws.ID, nil, "space", ws.OwnerID, input.Name, input.Description)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = response.JSON(w, http.StatusCreated, env)
	if err != nil {
		app.serverError(w, r, err)
	}
}

// listMyConnections returns all connections within a personal-space workspace.
// Personal space owner always has full access, so we always use ListConnections directly.
func (app *application) listMyConnections(w http.ResponseWriter, r *http.Request) {
	ws := contextGetWorkspace(r)
	conns, err := app.db.ListConnections(context.Background(), ws.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	err = response.JSON(w, http.StatusOK, conns)
	if err != nil {
		app.serverError(w, r, err)
	}
}

// createMyConnection creates a connection within a personal-space workspace.
// Passes nil orgID (no org for personal spaces).
func (app *application) createMyConnection(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name          string              `json:"name"`
		Driver        string              `json:"driver"`
		DSN           string              `json:"dsn"`
		EnvironmentID *int64              `json:"environment_id"`
		AccessMode    string              `json:"access_mode"`
		V             validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.V.CheckField(input.Name != "", "name", "name is required")
	input.V.CheckField(input.Driver != "", "driver", "driver is required")
	input.V.CheckField(input.DSN != "", "dsn", "dsn is required")
	if input.AccessMode == "" {
		input.AccessMode = "open"
	}
	input.V.CheckField(
		input.AccessMode == "open" || input.AccessMode == "restricted",
		"access_mode", "must be open or restricted",
	)
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	dsnEncrypted, err := encrypt.Encrypt(app.encKey, input.DSN)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	ws := contextGetWorkspace(r)
	conn, err := app.db.InsertConnection(context.Background(),
		ws.ID, input.EnvironmentID, nil,
		"space", ws.OwnerID,
		input.Name, input.Driver, dsnEncrypted, input.AccessMode,
	)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = response.JSON(w, http.StatusCreated, conn)
	if err != nil {
		app.serverError(w, r, err)
	}
}
