package main

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/driver"
	"github.com/sqlwarden/internal/encrypt"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

func (app *application) testConnection(w http.ResponseWriter, r *http.Request) {
	account := contextGetAccount(r)
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)

	if !app.enforcer.Can(account.ID, org.Slug, "workspace:"+ws.ID, "manage") {
		app.notPermitted(w, r)
		return
	}

	var input struct {
		Driver string `json:"driver"`
		DSN    string `json:"dsn"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	var v validator.Validator
	v.CheckField(validator.NotBlank(input.Driver), "driver", "Driver is required")
	v.CheckField(validator.In(input.Driver, "postgres", "mysql", "sqlite"), "driver", "Driver must be postgres, mysql, or sqlite")
	v.CheckField(validator.NotBlank(input.DSN), "dsn", "DSN is required")

	if v.HasErrors() {
		app.failedValidation(w, r, v)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	start := time.Now()

	d, err := driver.New(input.Driver)
	if err != nil {
		err = response.JSON(w, http.StatusOK, map[string]any{
			"ok":    false,
			"error": err.Error(),
		})
		if err != nil {
			app.serverError(w, r, err)
		}
		return
	}

	err = d.Connect(ctx, driver.ConnectionConfig{DSN: input.DSN, Driver: input.Driver})
	if err != nil {
		latency := time.Since(start).Milliseconds()
		err = response.JSON(w, http.StatusOK, map[string]any{
			"ok":         false,
			"latency_ms": latency,
			"error":      err.Error(),
		})
		if err != nil {
			app.serverError(w, r, err)
		}
		return
	}
	defer d.Close()

	err = d.Ping(ctx)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		err = response.JSON(w, http.StatusOK, map[string]any{
			"ok":         false,
			"latency_ms": latency,
			"error":      err.Error(),
		})
		if err != nil {
			app.serverError(w, r, err)
		}
		return
	}

	err = response.JSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"latency_ms": latency,
	})
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) listConnections(w http.ResponseWriter, r *http.Request) {
	ws := contextGetWorkspace(r)

	conns, err := app.db.GetConnectionsByWorkspace(ws.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	if conns == nil {
		conns = []database.Connection{}
	}

	err = response.JSON(w, http.StatusOK, conns)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) createConnection(w http.ResponseWriter, r *http.Request) {
	account := contextGetAccount(r)
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)

	if !app.enforcer.Can(account.ID, org.Slug, "workspace:"+ws.ID, "manage") {
		app.notPermitted(w, r)
		return
	}

	var input struct {
		Name   string `json:"name"`
		Driver string `json:"driver"`
		DSN    string `json:"dsn"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	var v validator.Validator
	v.CheckField(validator.NotBlank(input.Name), "name", "Name is required")
	v.CheckField(validator.MaxRunes(input.Name, 100), "name", "Must not be more than 100 characters")
	v.CheckField(validator.In(input.Driver, "postgres", "mysql", "sqlite"), "driver", "Driver must be postgres, mysql, or sqlite")
	v.CheckField(validator.NotBlank(input.DSN), "dsn", "DSN is required")

	if v.HasErrors() {
		app.failedValidation(w, r, v)
		return
	}

	encryptedDSN, err := encrypt.Encrypt(app.encKey, input.DSN)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	conn, err := app.db.InsertConnection(ws.ID, org.ID, input.Name, input.Driver, encryptedDSN)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = response.JSON(w, http.StatusCreated, conn)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getConnection(w http.ResponseWriter, r *http.Request) {
	account := contextGetAccount(r)
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)
	connID := chi.URLParam(r, "conn_id")

	conn, found, err := app.db.GetConnection(connID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found || conn.WorkspaceID != ws.ID {
		app.notFound(w, r)
		return
	}

	if !app.enforcer.CanOnConnection(account.ID, org.Slug, conn.ID, ws.ID, "connect") {
		app.notPermitted(w, r)
		return
	}

	err = response.JSON(w, http.StatusOK, conn)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) deleteConnection(w http.ResponseWriter, r *http.Request) {
	account := contextGetAccount(r)
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)
	connID := chi.URLParam(r, "conn_id")

	conn, found, err := app.db.GetConnection(connID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found || conn.WorkspaceID != ws.ID {
		app.notFound(w, r)
		return
	}

	if !app.enforcer.Can(account.ID, org.Slug, "workspace:"+ws.ID, "manage") {
		app.notPermitted(w, r)
		return
	}

	err = app.db.DeleteConnection(connID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (app *application) listConnectionOverrides(w http.ResponseWriter, r *http.Request) {
	account := contextGetAccount(r)
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)
	connID := chi.URLParam(r, "conn_id")

	if !app.enforcer.Can(account.ID, org.Slug, "workspace:"+ws.ID, "manage") {
		app.notPermitted(w, r)
		return
	}

	entries, err := app.enforcer.ListConnectionOverrides(org.Slug, connID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = response.JSON(w, http.StatusOK, entries)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) grantConnectionOverride(w http.ResponseWriter, r *http.Request) {
	account := contextGetAccount(r)
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)
	connID := chi.URLParam(r, "conn_id")

	if !app.enforcer.Can(account.ID, org.Slug, "workspace:"+ws.ID, "manage") {
		app.notPermitted(w, r)
		return
	}

	var input struct {
		Subject   string     `json:"subject"`
		Action    string     `json:"action"`
		ExpiresAt *time.Time `json:"expires_at"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	var v validator.Validator
	v.CheckField(validator.NotBlank(input.Subject), "subject", "Subject is required")
	v.CheckField(validator.In(input.Action, "connect", "query", "execute", "manage"), "action", "Action must be connect, query, execute, or manage")

	if v.HasErrors() {
		app.failedValidation(w, r, v)
		return
	}

	err = app.enforcer.GrantConnectionOverride(input.Subject, org.Slug, connID, input.Action)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	_, err = app.db.InsertAccessGrant(org.ID, input.Subject, "connection:"+connID, input.Action, account.ID, input.ExpiresAt)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (app *application) revokeConnectionOverride(w http.ResponseWriter, r *http.Request) {
	account := contextGetAccount(r)
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)
	connID := chi.URLParam(r, "conn_id")

	if !app.enforcer.Can(account.ID, org.Slug, "workspace:"+ws.ID, "manage") {
		app.notPermitted(w, r)
		return
	}

	subject := chi.URLParam(r, "subject")

	err := app.enforcer.RevokeConnectionOverride(subject, org.Slug, connID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = app.db.DeleteAccessGrant(subject, "connection:"+connID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (app *application) connectToDatabase(w http.ResponseWriter, r *http.Request) {
	account := contextGetAccount(r)
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)
	connID := chi.URLParam(r, "conn_id")

	conn, found, err := app.db.GetConnection(connID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found || conn.WorkspaceID != ws.ID {
		app.notFound(w, r)
		return
	}

	if !app.enforcer.CanOnConnection(account.ID, org.Slug, conn.ID, ws.ID, "connect") {
		app.notPermitted(w, r)
		return
	}

	plainDSN, err := encrypt.Decrypt(app.encKey, conn.DSN)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	session, created, err := app.connManager.GetOrCreate(account.ID, conn.ID, func() (driver.Driver, error) {
		d, err := driver.New(conn.Driver)
		if err != nil {
			return nil, err
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		err = d.Connect(ctx, driver.ConnectionConfig{DSN: plainDSN, Driver: conn.Driver})
		if err != nil {
			return nil, err
		}
		return d, nil
	})
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = response.JSON(w, http.StatusOK, map[string]any{
		"session_id": session.ID,
		"reused":     !created,
	})
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) executeQuery(w http.ResponseWriter, r *http.Request) {
	account := contextGetAccount(r)
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)
	connID := chi.URLParam(r, "conn_id")

	sessionID := r.Header.Get("X-Warden-Session")
	if sessionID == "" {
		app.errorMessage(w, r, http.StatusBadRequest, "X-Warden-Session header is required", nil)
		return
	}

	session, ok := app.connManager.Get(sessionID)
	if !ok {
		app.errorMessage(w, r, http.StatusGone, "Session has expired or does not exist", nil)
		return
	}

	if session.AccountID != account.ID {
		app.notPermitted(w, r)
		return
	}

	var input struct {
		SQL string `json:"sql"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	var v validator.Validator
	v.CheckField(validator.NotBlank(input.SQL), "sql", "SQL is required")

	if v.HasErrors() {
		app.failedValidation(w, r, v)
		return
	}

	trimmedUpper := strings.TrimSpace(strings.ToUpper(input.SQL))
	isSelect := strings.HasPrefix(trimmedUpper, "SELECT")

	if isSelect {
		if !app.enforcer.CanOnConnection(account.ID, org.Slug, connID, ws.ID, "query") {
			app.notPermitted(w, r)
			return
		}

		rs, err := session.Query(r.Context(), input.SQL)
		if err != nil {
			app.serverError(w, r, err)
			return
		}

		err = response.JSON(w, http.StatusOK, rs)
		if err != nil {
			app.serverError(w, r, err)
		}
	} else {
		if !app.enforcer.CanOnConnection(account.ID, org.Slug, connID, ws.ID, "execute") {
			app.notPermitted(w, r)
			return
		}

		rs, err := session.Execute(r.Context(), input.SQL)
		if err != nil {
			app.serverError(w, r, err)
			return
		}

		err = response.JSON(w, http.StatusOK, rs)
		if err != nil {
			app.serverError(w, r, err)
		}
	}
}
