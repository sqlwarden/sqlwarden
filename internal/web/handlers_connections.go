package web

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/driver"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/sqlquery"
	"github.com/sqlwarden/internal/validator"
	"github.com/sqlwarden/pkg/result"
)

func (app *application) validateConnectionEnvironment(r *http.Request, workspaceID int64, envID *int64) (*int64, bool, error) {
	if envID == nil {
		return nil, true, nil
	}

	env, found, err := app.db.GetEnvironment(r.Context(), *envID)
	if err != nil {
		return nil, false, err
	}
	if !found || env.WorkspaceID != workspaceID {
		return nil, false, nil
	}
	return &env.ID, true, nil
}

func (app *application) listConnections(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
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
		app.failedValidation(w, r, fieldErrors(map[string]string{"access_mode": "Access mode must be open or restricted."}))
		return
	}
	if env.ID != 0 {
		params.EnvironmentID = &env.ID
	} else if rawEnvID := strings.TrimSpace(r.URL.Query().Get("environment_id")); rawEnvID != "" {
		envID, err := strconv.ParseInt(rawEnvID, 10, 64)
		if err != nil || envID < 1 {
			app.failedValidation(w, r, fieldErrors(map[string]string{"environment_id": "Environment must be a positive integer."}))
			return
		}
		params.EnvironmentID = &envID
	}
	account := contextGetAccount(r)
	conns, err := app.db.ListAccessibleConnections(r.Context(), account.ID, org.ID, ws.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	result := filterAccessibleConnections(conns, params)

	err = response.JSON(w, http.StatusOK, result)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func filterAccessibleConnections(conns []database.Connection, params database.ListConnectionsParams) response.Paginated[database.Connection] {
	filtered := make([]database.Connection, 0, len(conns))
	search := strings.ToLower(strings.TrimSpace(params.Search))

	for _, conn := range conns {
		if search != "" && !strings.Contains(strings.ToLower(conn.Name), search) {
			continue
		}
		if params.EnvironmentID != nil {
			if conn.EnvironmentID != *params.EnvironmentID {
				continue
			}
		}
		if params.Driver != "" && conn.Driver != params.Driver {
			continue
		}
		if params.AccessMode != "" && conn.AccessMode != params.AccessMode {
			continue
		}
		filtered = append(filtered, conn)
	}

	sort.Slice(filtered, func(i, j int) bool {
		cmp := compareConnection(filtered[i], filtered[j], params.Sort)
		if params.Order == "asc" {
			return cmp < 0
		}
		return cmp > 0
	})

	total := len(filtered)
	start := (params.Page - 1) * params.PageSize
	if start > total {
		start = total
	}
	end := start + params.PageSize
	if end > total {
		end = total
	}

	return response.Paginated[database.Connection]{
		Items:    filtered[start:end],
		Page:     params.Page,
		PageSize: params.PageSize,
		Total:    total,
	}
}

func compareConnection(left, right database.Connection, sortBy string) int {
	switch sortBy {
	case "name":
		if left.Name != right.Name {
			return strings.Compare(left.Name, right.Name)
		}
	case "driver":
		if left.Driver != right.Driver {
			return strings.Compare(left.Driver, right.Driver)
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

func queryLogAttrs(account database.Account, org database.Organization, ws database.Workspace, conn database.Connection, classification sqlquery.ClassifyResult) []any {
	return []any{
		slog.Group("account", "id", account.ID),
		slog.Group("org", "id", org.ID, "slug", org.Slug),
		slog.Group("workspace", "id", ws.ID, "owner_type", ws.OwnerType),
		slog.Group("connection", "id", conn.ID, "driver", conn.Driver),
		slog.Group("query", "kind", classification.Kind, "classifier", classification.Source, "diagnostics", len(classification.Diagnostics)),
	}
}

func (app *application) hasAnyConnectionRuntimePermission(r *http.Request, orgID int64, ownerType string, connectionID int64, permissions ...string) bool {
	for _, permission := range permissions {
		if app.hasConnectionPermission(r, orgID, ownerType, connectionID, permission) {
			return true
		}
	}
	return false
}

func (app *application) hasConnectionPermission(r *http.Request, orgID int64, ownerType string, connectionID int64, permission string) bool {
	account := contextGetAccount(r)
	return app.enforcer.Can(r.Context(), account.ID, orgID, ownerType, "connection", connectionID, permission)
}

func (app *application) classifyConnectionSQL(r *http.Request, conn database.Connection, sql string) (sqlquery.ClassifyResult, error) {
	return sqlquery.Classify(r.Context(), sqlquery.ClassifyRequest{
		RequestMetadata: sqlquery.RequestMetadata{
			Dialect: driver.Dialect(driver.NormalizeName(conn.Driver)),
		},
		SQL: sql,
	})
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

	input.V.CheckField(input.Name != "", "name", "Name is required.")
	input.V.CheckField(input.Driver != "", "driver", "Driver is required.")
	input.V.CheckField(input.DSN != "", "dsn", "DSN is required.")
	if input.Driver != "" {
		if err := app.validateTargetConnection(input.Driver, input.DSN); err != nil {
			if errors.Is(err, errSQLiteTargetDisabled) {
				app.logWarn(r, "sqlite target connection blocked", slog.String("operation", "create_connection"), slog.String("driver", input.Driver))
			}
			input.V.CheckField(false, "driver", targetConnectionFieldError(err))
		}
	}
	if input.AccessMode == "" {
		input.AccessMode = "open"
	}
	input.V.CheckField(
		input.AccessMode == "open" || input.AccessMode == "restricted",
		"access_mode", "Access mode must be open or restricted.",
	)

	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	dsnEncrypted, err := app.keyring.Encrypt(input.DSN)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	ws := contextGetWorkspace(r)
	env := contextGetEnvironment(r)
	targetEnvID := input.EnvironmentID
	if env.ID != 0 {
		targetEnvID = &env.ID
	} else {
		var ok bool
		targetEnvID, ok, err = app.validateConnectionEnvironment(r, ws.ID, targetEnvID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if !ok {
			app.notFound(w, r)
			return
		}
	}

	conn, err := app.db.InsertConnection(context.Background(),
		ws.ID, targetEnvID,
		input.Name, input.Driver, dsnEncrypted, input.AccessMode,
	)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.logInfo(r, "connection created", slog.Int64("workspace_id", ws.ID), slog.Int64("connection_id", conn.ID), slog.String("driver", conn.Driver), slog.String("access_mode", conn.AccessMode))
	err = response.JSON(w, http.StatusCreated, conn)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getConnection(w http.ResponseWriter, r *http.Request) {
	conn := contextGetConnection(r)
	ws := contextGetWorkspace(r)
	if ws.OwnerType == "org" {
		account := contextGetAccount(r)
		org := contextGetOrg(r)
		ok, err := app.db.HasAccessibleConnection(r.Context(), account.ID, org.ID, ws.ID, conn.ID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if !ok {
			app.notFound(w, r)
			return
		}
	}
	err := response.JSON(w, http.StatusOK, conn)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) updateConnection(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name       string              `json:"name"`
		Driver     *string             `json:"driver"`
		DSN        string              `json:"dsn"`
		AccessMode string              `json:"access_mode"`
		Force      bool                `json:"force"`
		V          validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.V.CheckField(input.Name != "", "name", "Name is required.")
	input.V.CheckField(input.DSN != "", "dsn", "DSN is required.")
	input.V.CheckField(input.Driver == nil, "driver", "Driver cannot be changed.")
	if input.AccessMode == "" {
		input.AccessMode = "open"
	}
	input.V.CheckField(
		input.AccessMode == "open" || input.AccessMode == "restricted",
		"access_mode", "Access mode must be open or restricted.",
	)
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	conn := contextGetConnection(r)
	if err := app.validateTargetConnection(conn.Driver, input.DSN); err != nil {
		if errors.Is(err, errSQLiteTargetDisabled) {
			app.logWarn(r, "sqlite target connection blocked", slog.String("operation", "update_connection"), slog.Int64("connection_id", conn.ID), slog.String("driver", conn.Driver))
		}
		v := validator.Validator{}
		v.AddFieldError("driver", targetConnectionFieldError(err))
		app.failedValidation(w, r, v)
		return
	}

	dsnEncrypted, err := app.keyring.Encrypt(input.DSN)
	if err != nil {
		app.errorMessage(w, r, http.StatusUnprocessableEntity, err.Error(), nil)
		return
	}

	currentDSN, err := app.keyring.Decrypt(conn.DSNEncrypted)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	dsnChanged := currentDSN != input.DSN
	if dsnChanged {
		activeSessions := app.connManager.CountForConnection(strconv.FormatInt(conn.ID, 10))
		if activeSessions > 0 && !input.Force {
			app.errorMessage(w, r, http.StatusConflict, "Connection has active sessions. Retry with force=true to rotate the DSN and drop them.", nil)
			return
		}
		if input.Force && activeSessions > 0 {
			app.connManager.RemoveForConnection(strconv.FormatInt(conn.ID, 10))
			app.logInfo(r, "connection sessions dropped for dsn rotation", slog.Int64("connection_id", conn.ID), slog.Int("dropped_sessions", activeSessions))
		}
	}
	err = app.db.UpdateConnection(r.Context(), conn.ID, input.Name, dsnEncrypted, input.AccessMode)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	app.logInfo(r, "connection updated", slog.Int64("connection_id", conn.ID), slog.Bool("dsn_rotated", dsnChanged), slog.String("access_mode", input.AccessMode))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) deleteConnection(w http.ResponseWriter, r *http.Request) {
	conn := contextGetConnection(r)
	err := app.db.DeleteConnection(context.Background(), conn.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	app.enforcer.InvalidateAncestry("connection", conn.ID)
	app.logInfo(r, "connection deleted", slog.Int64("connection_id", conn.ID), slog.Int64("workspace_id", conn.WorkspaceID), slog.String("driver", conn.Driver))
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

	input.V.CheckField(input.Driver != "", "driver", "Driver is required.")
	input.V.CheckField(input.DSN != "", "dsn", "DSN is required.")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}
	if err := app.validateTargetConnection(input.Driver, input.DSN); err != nil {
		if errors.Is(err, errSQLiteTargetDisabled) {
			app.logWarn(r, "sqlite target connection blocked", slog.String("operation", "test_connection"), slog.String("driver", input.Driver))
		}
		v := validator.Validator{}
		v.AddFieldError("driver", targetConnectionFieldError(err))
		app.failedValidation(w, r, v)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	start := time.Now()

	d, err := driver.New(input.Driver)
	if err != nil {
		app.logWarn(r, "connection test failed", slog.String("driver", input.Driver), slog.Int64("latency_ms", time.Since(start).Milliseconds()), slog.String("stage", "driver_init"), slog.String("error_category", connectionTestErrorCategory(err)))
		err = response.JSON(w, http.StatusUnprocessableEntity, map[string]any{
			"ok":    false,
			"error": err.Error(),
		})
		if err != nil {
			app.serverError(w, r, err)
		}
		return
	}

	err = d.Connect(ctx, app.driverConnectionConfig(input.Driver, input.DSN))
	if err != nil {
		latency := time.Since(start).Milliseconds()
		app.logWarn(r, "connection test failed", slog.String("driver", input.Driver), slog.Int64("latency_ms", latency), slog.String("stage", "connect"), slog.String("error_category", connectionTestErrorCategory(err)))
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
		app.logWarn(r, "connection test failed", slog.String("driver", input.Driver), slog.Int64("latency_ms", latency), slog.String("stage", "ping"), slog.String("error_category", connectionTestErrorCategory(err)))
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

	app.logInfo(r, "connection test completed", slog.String("driver", input.Driver), slog.Int64("latency_ms", latency), slog.Bool("ok", true))
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
	ws := contextGetWorkspace(r)

	allowed := app.hasAnyConnectionRuntimePermission(r, org.ID, ws.OwnerType, conn.ID,
		access.PermConnExecute,
		access.PermConnDQL,
		access.PermConnDML,
		access.PermConnDDL,
	)
	if !allowed {
		app.notPermitted(w, r)
		return
	}

	plainDSN, err := app.keyring.Decrypt(conn.DSNEncrypted)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if err := app.validateTargetConnection(conn.Driver, plainDSN); err != nil {
		app.errorMessage(w, r, http.StatusUnprocessableEntity, targetConnectionFieldError(err), nil)
		return
	}

	connID := strconv.FormatInt(conn.ID, 10)
	accountID := strconv.FormatInt(account.ID, 10)

	session, created, err := app.connManager.GetOrCreateWithMetadata(accountID, connID, connection.SessionMetadata{
		OrgID:       strconv.FormatInt(org.ID, 10),
		WorkspaceID: strconv.FormatInt(ws.ID, 10),
	}, func() (driver.Driver, error) {
		d, err := driver.New(conn.Driver)
		if err != nil {
			return nil, err
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		err = d.Connect(ctx, app.driverConnectionConfig(conn.Driver, plainDSN))
		if err != nil {
			return nil, err
		}
		return d, nil
	})
	if err != nil {
		app.errorMessage(w, r, http.StatusUnprocessableEntity, err.Error(), nil)
		return
	}

	app.logInfo(r, "database session opened", slog.Int64("connection_id", conn.ID), slog.String("session_id", session.ID), slog.Bool("reused", !created))
	err = response.JSON(w, http.StatusOK, map[string]any{
		"session_id": session.ID,
		"reused":     !created,
	})
	if err != nil {
		app.serverError(w, r, err)
	}
}

func connectionTestErrorCategory(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, errSQLiteTargetDisabled):
		return "policy_denied"
	case strings.Contains(err.Error(), "unknown driver"):
		return "unsupported_driver"
	default:
		return "target_unreachable"
	}
}

func (app *application) listActiveSessions(w http.ResponseWriter, r *http.Request) {
	account := contextGetAccount(r)
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)

	accountID := strconv.FormatInt(account.ID, 10)
	workspaceID := strconv.FormatInt(ws.ID, 10)

	type sessionInfo struct {
		ConnectionID int64  `json:"connection_id"`
		AccountID    int64  `json:"account_id"`
		SessionID    string `json:"session_id"`
	}
	result := make([]sessionInfo, 0)

	refs := app.connManager.AllForAccount(accountID)
	if org.ID != 0 && app.enforcer.Can(r.Context(), account.ID, org.ID, ws.OwnerType, "workspace", ws.ID, access.PermPolicyRead) {
		refs = app.connManager.AllForWorkspace(workspaceID)
	}

	for _, ref := range refs {
		if ref.WorkspaceID != "" && ref.WorkspaceID != workspaceID {
			continue
		}
		connIDInt, parseErr := strconv.ParseInt(ref.ConnectionID, 10, 64)
		if parseErr != nil {
			continue
		}
		accountIDInt, parseErr := strconv.ParseInt(ref.AccountID, 10, 64)
		if parseErr != nil {
			continue
		}
		result = append(result, sessionInfo{
			ConnectionID: connIDInt,
			AccountID:    accountIDInt,
			SessionID:    ref.SessionID,
		})
	}

	err := response.JSON(w, http.StatusOK, map[string]any{"sessions": result})
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) disconnectFromDatabase(w http.ResponseWriter, r *http.Request) {
	account := contextGetAccount(r)
	conn := contextGetConnection(r)

	sessionID := r.Header.Get("X-Warden-Session")
	if sessionID == "" {
		app.badRequest(w, r, errors.New("X-Warden-Session header is required"))
		return
	}

	session, ok := app.connManager.Get(sessionID)
	if !ok {
		// Session already gone (expired or never existed) — idempotent.
		w.WriteHeader(http.StatusNoContent)
		return
	}

	accountID := strconv.FormatInt(account.ID, 10)
	if session.AccountID != accountID {
		app.notPermitted(w, r)
		return
	}

	connID := strconv.FormatInt(conn.ID, 10)
	if session.ConnectionID != connID {
		app.badRequest(w, r, errors.New("session does not belong to this connection"))
		return
	}

	app.connManager.Remove(sessionID)
	app.logInfo(r, "database session disconnected", slog.Int64("connection_id", conn.ID), slog.String("session_id", sessionID))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) revokeWorkspaceDatabaseSession(w http.ResponseWriter, r *http.Request) {
	account := contextGetAccount(r)
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)
	sessionID := strings.TrimSpace(chi.URLParam(r, "session_id"))
	if sessionID == "" {
		app.notFound(w, r)
		return
	}

	session, ok := app.connManager.Get(sessionID)
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	workspaceID := strconv.FormatInt(ws.ID, 10)
	if session.WorkspaceID != workspaceID {
		app.notFound(w, r)
		return
	}

	accountID := strconv.FormatInt(account.ID, 10)
	if session.AccountID != accountID {
		if org.ID == 0 || !app.enforcer.Can(r.Context(), account.ID, org.ID, ws.OwnerType, "workspace", ws.ID, access.PermPolicyModify) {
			app.notPermitted(w, r)
			return
		}
	}

	app.connManager.Remove(sessionID)
	app.logInfo(r, "database session revoked", slog.Int64("workspace_id", ws.ID), slog.String("session_id", sessionID), slog.String("session_account_id", session.AccountID))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) driverConnectionConfig(driverName, dsn string) driver.ConnectionConfig {
	return driver.ConnectionConfig{
		DSN:            dsn,
		Driver:         driverName,
		MaxResultRows:  app.config.Query.MaxResultRows,
		MaxResultBytes: int64(app.config.Query.MaxResultBytes),
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

	input.V.CheckField(input.SQL != "", "sql", "SQL is required.")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	account := contextGetAccount(r)
	org := contextGetOrg(r)
	conn := contextGetConnection(r)
	ws := contextGetWorkspace(r)

	sessionID := r.Header.Get("X-Warden-Session")
	if sessionID == "" {
		app.errorMessage(w, r, http.StatusBadRequest, "X-Warden-Session header is required.", nil)
		return
	}

	session, ok := app.connManager.Get(sessionID)
	if !ok {
		app.errorMessage(w, r, http.StatusGone, "Session has expired or does not exist.", nil)
		return
	}

	if session.AccountID != strconv.FormatInt(account.ID, 10) {
		app.notPermitted(w, r)
		return
	}
	if session.ConnectionID != strconv.FormatInt(conn.ID, 10) {
		app.notPermitted(w, r)
		return
	}

	hasBroadExecute := app.hasConnectionPermission(r, org.ID, ws.OwnerType, conn.ID, access.PermConnExecute)
	classification, err := app.classifyConnectionSQL(r, conn, input.SQL)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	logAttrs := queryLogAttrs(account, org, ws, conn, classification)
	if classification.Kind == sqlquery.KindUnknown {
		app.logger.Warn("query classification unknown", logAttrs...)
	} else {
		app.logger.Debug("query classified", logAttrs...)
	}

	var rs *result.ResultSet
	var execErr error
	start := time.Now()

	switch classification.Kind {
	case sqlquery.KindDQL:
		if !hasBroadExecute && !app.enforcer.Can(r.Context(),
			account.ID, org.ID,
			ws.OwnerType, "connection", conn.ID,
			access.PermConnDQL,
		) {
			app.logger.Warn("query permission denied", append(logAttrs, "required_permission", access.PermConnDQL)...)
			app.notPermitted(w, r)
			return
		}
		rs, execErr = session.Query(r.Context(), input.SQL)
	case sqlquery.KindDML:
		if !hasBroadExecute && !app.enforcer.Can(r.Context(),
			account.ID, org.ID,
			ws.OwnerType, "connection", conn.ID,
			access.PermConnDML,
		) {
			app.logger.Warn("query permission denied", append(logAttrs, "required_permission", access.PermConnDML)...)
			app.notPermitted(w, r)
			return
		}
		rs, execErr = session.Execute(r.Context(), input.SQL)
	case sqlquery.KindDDL:
		if !hasBroadExecute && !app.enforcer.Can(r.Context(),
			account.ID, org.ID,
			ws.OwnerType, "connection", conn.ID,
			access.PermConnDDL,
		) {
			app.logger.Warn("query permission denied", append(logAttrs, "required_permission", access.PermConnDDL)...)
			app.notPermitted(w, r)
			return
		}
		rs, execErr = session.Execute(r.Context(), input.SQL)
	default:
		if !hasBroadExecute {
			app.logger.Warn("query permission denied", append(logAttrs, "required_permission", access.PermConnExecute)...)
			app.notPermitted(w, r)
			return
		}
		rs, execErr = session.Execute(r.Context(), input.SQL)
	}

	if execErr != nil {
		if errors.Is(execErr, context.Canceled) || errors.Is(execErr, context.DeadlineExceeded) || r.Context().Err() != nil {
			app.connManager.Remove(sessionID)
			app.logger.Warn("query cancelled", append(logAttrs, "duration_ms", time.Since(start).Milliseconds())...)
			app.errorMessage(w, r, statusClientClosedRequest, "Query was cancelled.", nil)
			return
		}
		app.logger.Warn("query execution failed", append(logAttrs, "duration_ms", time.Since(start).Milliseconds(), "error", execErr.Error())...)
		app.errorMessage(w, r, http.StatusUnprocessableEntity, execErr.Error(), nil)
		return
	}

	rs.DurationMs = time.Since(start).Milliseconds()
	app.logger.Info("query executed", append(logAttrs,
		"duration_ms", rs.DurationMs,
		slog.Group("result", "rows", len(rs.Rows), "columns", len(rs.Columns)),
	)...)

	err = response.JSON(w, http.StatusOK, rs)
	if err != nil {
		app.serverError(w, r, err)
	}
}
