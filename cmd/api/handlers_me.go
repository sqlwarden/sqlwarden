package main

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/driver"
	"github.com/sqlwarden/internal/encrypt"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

// listMyWorkspaces returns all personal-space workspaces owned by the authenticated account.
func (app *application) listMyWorkspaces(w http.ResponseWriter, r *http.Request) {
	account := contextGetAccount(r)
	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"name":       "name",
		"created_at": "created_at",
	})
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}

	wss, err := app.db.ListWorkspacesByOwner(r.Context(), "space", account.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	wss = filterAccessibleWorkspaces(wss, q.Search, name, q.Sort, q.Order)
	result := database.PaginateItems(wss, q.Page, q.PageSize)

	err = response.JSON(w, http.StatusOK, result)
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

// listMyEnvironments returns all environments within a personal-space workspace.
// Personal space owner has unconditional access, so all environments are returned.
func (app *application) listMyEnvironments(w http.ResponseWriter, r *http.Request) {
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

	envs, err := app.db.ListEnvironmentsPage(r.Context(), database.ListEnvironmentsParams{
		WorkspaceID: ws.ID,
		Search:      q.Search,
		Name:        name,
		Sort:        q.Sort,
		Order:       q.Order,
		Page:        q.Page,
		PageSize:    q.PageSize,
	})
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	err = response.JSON(w, http.StatusOK, envs)
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
		if isUniqueViolation(err) {
			input.V.AddFieldError("name", "an environment with this name already exists in this workspace")
			app.failedValidation(w, r, input.V)
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

// listMyConnections returns all connections within a personal-space workspace.
// Personal space owner always has full access, so we always use ListConnections directly.
func (app *application) listMyConnections(w http.ResponseWriter, r *http.Request) {
	ws := contextGetWorkspace(r)
	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"name":       "name",
		"created_at": "created_at",
		"driver":     "driver",
	})
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}

	params := database.ListConnectionsParams{
		WorkspaceID: ws.ID,
		Search:      q.Search,
		Driver:      strings.TrimSpace(r.URL.Query().Get("driver")),
		AccessMode:  strings.TrimSpace(r.URL.Query().Get("access_mode")),
		Sort:        q.Sort,
		Order:       q.Order,
		Page:        q.Page,
		PageSize:    q.PageSize,
	}
	if params.AccessMode != "" && params.AccessMode != "open" && params.AccessMode != "restricted" {
		app.failedValidation(w, r, fieldErrors(map[string]string{"access_mode": "must be open or restricted"}))
		return
	}
	if rawEnvID := strings.TrimSpace(r.URL.Query().Get("environment_id")); rawEnvID != "" {
		envID, err := strconv.ParseInt(rawEnvID, 10, 64)
		if err != nil || envID < 1 {
			app.failedValidation(w, r, fieldErrors(map[string]string{"environment_id": "must be a positive integer"}))
			return
		}
		params.EnvironmentID = &envID
	}

	conns, err := app.db.ListConnectionsPage(context.Background(), params)
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
	if input.Driver != "" {
		if _, err := driver.New(input.Driver); err != nil {
			input.V.CheckField(false, "driver", "must be a supported driver")
		}
	}
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
	validatedEnvID, ok, err := app.resolveWorkspaceEnvironmentID(r, ws.ID, input.EnvironmentID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !ok {
		app.notFound(w, r)
		return
	}

	conn, err := app.db.InsertConnection(context.Background(),
		ws.ID, validatedEnvID, nil,
		"space", ws.OwnerID,
		input.Name, input.Driver, dsnEncrypted, input.AccessMode,
	)
	if err != nil {
		if isForeignKeyViolation(err) {
			app.notFound(w, r)
			return
		}
		app.serverError(w, r, err)
		return
	}

	err = response.JSON(w, http.StatusCreated, conn)
	if err != nil {
		app.serverError(w, r, err)
	}
}
