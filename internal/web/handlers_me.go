package web

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/sqlwarden/internal/catalog"
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

	wss, err := app.catalogService().ListPersonalWorkspaces(r.Context(), account.ID, catalog.ListQuery{
		Search: q.Search, Name: name, Sort: q.Sort, Order: q.Order, Page: q.Page, PageSize: q.PageSize,
	})
	if err != nil {
		app.catalogError(w, r, err)
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
		Name        string `json:"name"`
		Description string `json:"description"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	account := contextGetAccount(r)
	ws, err := app.catalogService().CreatePersonalWorkspace(r.Context(), catalog.Actor{AccountID: account.ID}, catalog.CreateWorkspaceInput{
		Name: input.Name, Description: input.Description,
	})
	if err != nil {
		if errors.Is(err, catalog.ErrNameTaken) {
			app.failedDuplicateField(w, r, "name", "A workspace with this name already exists.")
			return
		}
		app.catalogError(w, r, err)
		return
	}

	app.logInfo(r, "personal workspace created", slog.Int64("workspace_id", ws.ID), slog.Int64("owner_account_id", account.ID))
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

	envs, err := app.catalogService().ListWorkspaceEnvironments(r.Context(), ws.ID, catalog.ListQuery{
		Search: q.Search, Name: name, Sort: q.Sort, Order: q.Order, Page: q.Page, PageSize: q.PageSize,
	})
	if err != nil {
		app.catalogError(w, r, err)
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
		Name        string `json:"name"`
		Description string `json:"description"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	ws := contextGetWorkspace(r)
	env, err := app.catalogService().CreateEnvironment(r.Context(), catalogActor(r), ws.ID, catalog.CreateEnvironmentInput{
		Name: input.Name, Description: input.Description,
	})
	if err != nil {
		if errors.Is(err, catalog.ErrNameTaken) {
			app.failedDuplicateField(w, r, "name", "An environment with this name already exists in this workspace.")
			return
		}
		app.catalogError(w, r, err)
		return
	}

	app.logInfo(r, "personal workspace environment created", slog.Int64("workspace_id", ws.ID), slog.Int64("environment_id", env.ID))
	err = response.JSON(w, http.StatusCreated, env)
	if err != nil {
		app.serverError(w, r, err)
	}
}

// listMyConnections returns all connections within a personal-space environment.
func (app *application) listMyConnections(w http.ResponseWriter, r *http.Request) {
	ws := contextGetWorkspace(r)
	env := contextGetEnvironment(r)
	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"name":       "name",
		"created_at": "created_at",
		"driver":     "driver",
	})
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}

	query := catalog.ListConnectionsQuery{
		ListQuery:     catalog.ListQuery{Search: q.Search, Sort: q.Sort, Order: q.Order, Page: q.Page, PageSize: q.PageSize},
		EnvironmentID: &env.ID,
		Driver:        strings.TrimSpace(r.URL.Query().Get("driver")),
		AccessMode:    strings.TrimSpace(r.URL.Query().Get("access_mode")),
	}
	if query.AccessMode != "" && query.AccessMode != catalog.AccessModeOpen && query.AccessMode != catalog.AccessModeRestricted {
		app.failedValidation(w, r, fieldErrors(map[string]string{"access_mode": "Access mode must be open or restricted."}))
		return
	}
	if env.ID == 0 {
		if rawEnvID := strings.TrimSpace(r.URL.Query().Get("environment_id")); rawEnvID != "" {
			parsedEnvID, err := strconv.ParseInt(rawEnvID, 10, 64)
			if err != nil || parsedEnvID < 1 {
				app.failedValidation(w, r, fieldErrors(map[string]string{"environment_id": "Environment must be a positive integer."}))
				return
			}
			query.EnvironmentID = &parsedEnvID
		}
	}

	conns, err := app.catalogService().ListWorkspaceConnections(r.Context(), ws.ID, query)
	if err != nil {
		app.catalogError(w, r, err)
		return
	}
	err = response.JSON(w, http.StatusOK, conns)
	if err != nil {
		app.serverError(w, r, err)
	}
}

// createMyConnection creates a connection within a personal-space environment.
func (app *application) createMyConnection(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name          string              `json:"name"`
		Driver        string              `json:"driver"`
		DSN           string              `json:"dsn"`
		EnvironmentID *int64              `json:"environment_id"`
		AccessMode    string              `json:"access_mode"`
		TLS           *tlsConfigDocument  `json:"tls"`
		SSH           *sshConfigDocument  `json:"ssh"`
		V             validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	var tlsDoc tlsConfigDocument
	if input.TLS != nil {
		tlsDoc = *input.TLS
		app.validateTLSDocument(input.Driver, tlsDoc, &input.V)
	}
	var sshDoc sshConfigDocument
	if input.SSH != nil {
		sshDoc = *input.SSH
		app.validateSSHDocument(input.Driver, sshDoc, &input.V)
	}
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	tlsEncrypted, err := app.sealTLSDocument(tlsDoc)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	sshEncrypted, err := app.sealSSHDocument(sshDoc)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	ws := contextGetWorkspace(r)
	env := contextGetEnvironment(r)
	targetEnvID := input.EnvironmentID
	if env.ID != 0 {
		targetEnvID = &env.ID
	}

	conn, err := app.catalogService().CreateConnection(r.Context(), catalogActor(r), ws.ID, catalog.CreateConnectionInput{
		Name: input.Name, Driver: input.Driver, DSN: input.DSN,
		EnvironmentID: targetEnvID, AccessMode: input.AccessMode,
		SealedTLS: tlsEncrypted, SealedSSH: sshEncrypted,
	})
	if err != nil {
		app.catalogError(w, r, err)
		return
	}

	if tlsEncrypted != "" {
		app.logInfo(r, "connection tls configured", slog.Int64("connection_id", conn.ID))
	}

	if sshEncrypted != "" {
		app.logInfo(r, "connection ssh configured", slog.Int64("connection_id", conn.ID))
	}

	app.logInfo(r, "personal workspace connection created",
		slog.Int64("workspace_id", ws.ID),
		slog.Int64("connection_id", conn.ID),
		slog.String("driver", conn.Driver),
		slog.String("access_mode", conn.AccessMode),
	)
	err = response.JSON(w, http.StatusCreated, conn)
	if err != nil {
		app.serverError(w, r, err)
	}
}
