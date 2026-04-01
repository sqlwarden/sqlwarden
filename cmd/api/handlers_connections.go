package main

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/driver"
	"github.com/sqlwarden/internal/encrypt"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

func (app *application) listConnections(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)

	var (
		conns []database.Connection
		err   error
	)
	if app.config.desktopMode {
		conns, err = app.db.ListConnections(context.Background(), ws.ID)
	} else {
		account := contextGetAccount(r)
		conns, err = app.db.ListAccessibleConnections(context.Background(), account.ID, org.ID, ws.ID)
	}
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = response.JSON(w, http.StatusOK, conns)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) createConnection(w http.ResponseWriter, r *http.Request) {
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

	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)
	conn, err := app.db.InsertConnection(context.Background(), 
		ws.ID, input.EnvironmentID, &org.ID,
		ws.OwnerType, ws.OwnerID,
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

func (app *application) getConnection(w http.ResponseWriter, r *http.Request) {
	conn := contextGetConnection(r)
	err := response.JSON(w, http.StatusOK, conn)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) deleteConnection(w http.ResponseWriter, r *http.Request) {
	conn := contextGetConnection(r)
	err := app.db.DeleteConnection(context.Background(), conn.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	app.enforcer.InvalidateAncestry("connection", conn.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) testConnection(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Driver string              `json:"driver"`
		DSN    string              `json:"dsn"`
		V      validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.V.CheckField(input.Driver != "", "driver", "driver is required")
	input.V.CheckField(input.DSN != "", "dsn", "dsn is required")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	start := time.Now()

	d, err := driver.New(input.Driver)
	if err != nil {
		err = response.JSON(w, http.StatusUnprocessableEntity, map[string]any{
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

func (app *application) connectToDatabase(w http.ResponseWriter, r *http.Request) {
	account := contextGetAccount(r)
	org := contextGetOrg(r)
	conn := contextGetConnection(r)

	allowed := app.enforcer.Can(r.Context(),
		account.ID, org.ID,
		conn.OwnerType, "connection", conn.ID,
		access.PermConnExecute,
	)
	if !allowed {
		app.notPermitted(w, r)
		return
	}

	plainDSN, err := encrypt.Decrypt(app.encKey, conn.DSNEncrypted)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	connID := strconv.FormatInt(conn.ID, 10)
	accountID := strconv.FormatInt(account.ID, 10)

	session, created, err := app.connManager.GetOrCreate(accountID, connID, func() (driver.Driver, error) {
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
	var input struct {
		SQL string              `json:"sql"`
		V   validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.V.CheckField(input.SQL != "", "sql", "sql is required")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	account := contextGetAccount(r)
	org := contextGetOrg(r)
	conn := contextGetConnection(r)

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

	if session.AccountID != strconv.FormatInt(account.ID, 10) {
		app.notPermitted(w, r)
		return
	}

	trimmedUpper := strings.TrimSpace(strings.ToUpper(input.SQL))
	isSelect := strings.HasPrefix(trimmedUpper, "SELECT")

	if isSelect {
		allowed := app.enforcer.Can(r.Context(),
			account.ID, org.ID,
			conn.OwnerType, "connection", conn.ID,
			access.PermQueryExecute,
		)
		if !allowed {
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
		allowed := app.enforcer.Can(r.Context(),
			account.ID, org.ID,
			conn.OwnerType, "connection", conn.ID,
			access.PermConnExecute,
		)
		if !allowed {
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

func (app *application) listConnectionBindings(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	conn := contextGetConnection(r)

	rbs, err := app.db.ListRoleBindings(context.Background(), org.ID, "connection", conn.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	pbs, err := app.db.ListPermissionBindings(context.Background(), org.ID, "connection", conn.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = response.JSON(w, http.StatusOK, map[string]any{
		"role_bindings":       rbs,
		"permission_bindings": pbs,
	})
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) grantConnectionAccess(w http.ResponseWriter, r *http.Request) {
	var input struct {
		RoleID      int64               `json:"role_id"`
		Permissions []string            `json:"permissions"`
		SubjectType string              `json:"subject_type"`
		SubjectID   int64               `json:"subject_id"`
		V           validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	hasRole := input.RoleID > 0
	hasPerms := len(input.Permissions) > 0
	input.V.CheckField(hasRole || hasPerms, "role_id", "one of role_id or permissions is required")
	input.V.CheckField(!(hasRole && hasPerms), "role_id", "only one of role_id or permissions may be set")
	input.V.CheckField(input.SubjectType == "account" || input.SubjectType == "team", "subject_type", "must be account or team")
	input.V.CheckField(input.SubjectID > 0, "subject_id", "subject_id is required")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	org := contextGetOrg(r)
	conn := contextGetConnection(r)
	grantor := contextGetAccount(r)

	if hasRole {
		err = app.enforcer.BindRole(r.Context(), org.ID, input.RoleID, input.SubjectType, input.SubjectID, "connection", conn.ID, grantor.ID)
	} else {
		err = app.enforcer.GrantPermissions(r.Context(), org.ID, input.Permissions, input.SubjectType, input.SubjectID, "connection", conn.ID, grantor.ID)
	}
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (app *application) revokeConnectionAccess(w http.ResponseWriter, r *http.Request) {
	bindingIDStr := chi.URLParam(r, "binding_id")
	bindingID, err := strconv.ParseInt(bindingIDStr, 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}

	org := contextGetOrg(r)
	kind := r.URL.Query().Get("kind")

	if kind == "permission" {
		err = app.enforcer.RevokePermission(r.Context(), bindingID, org.ID)
	} else {
		err = app.enforcer.UnbindRole(r.Context(), bindingID, org.ID)
	}
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
