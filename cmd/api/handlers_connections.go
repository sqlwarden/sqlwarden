package main

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/driver"
	"github.com/sqlwarden/internal/encrypt"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
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
		app.failedValidation(w, r, fieldErrors(map[string]string{"access_mode": "must be open or restricted"}))
		return
	}
	if env.ID != 0 {
		params.EnvironmentID = &env.ID
	} else if rawEnvID := strings.TrimSpace(r.URL.Query().Get("environment_id")); rawEnvID != "" {
		envID, err := strconv.ParseInt(rawEnvID, 10, 64)
		if err != nil || envID < 1 {
			app.failedValidation(w, r, fieldErrors(map[string]string{"environment_id": "must be a positive integer"}))
			return
		}
		params.EnvironmentID = &envID
	}
	var (
		result response.Paginated[database.Connection]
		err    error
	)
	if app.config.desktopMode {
		result, err = app.db.ListConnectionsPage(context.Background(), params)
	} else {
		account := contextGetAccount(r)
		conns, err := app.db.ListAccessibleConnections(context.Background(), account.ID, org.ID, ws.ID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		result = filterAccessibleConnections(conns, params)
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

	err = response.JSON(w, http.StatusCreated, conn)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getConnection(w http.ResponseWriter, r *http.Request) {
	conn := contextGetConnection(r)
	ws := contextGetWorkspace(r)
	if ws.OwnerType == "org" && !app.config.desktopMode {
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
		V          validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.V.CheckField(input.Name != "", "name", "name is required")
	input.V.CheckField(input.DSN != "", "dsn", "dsn is required")
	input.V.CheckField(input.Driver == nil, "driver", "driver cannot be changed")
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
		app.errorMessage(w, r, http.StatusUnprocessableEntity, err.Error(), nil)
		return
	}

	conn := contextGetConnection(r)
	err = app.db.UpdateConnection(r.Context(), conn.ID, input.Name, dsnEncrypted, input.AccessMode)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
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
	ws := contextGetWorkspace(r)

	allowed := app.enforcer.Can(r.Context(),
		account.ID, org.ID,
		ws.OwnerType, "connection", conn.ID,
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
		app.errorMessage(w, r, http.StatusUnprocessableEntity, err.Error(), nil)
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
	ws := contextGetWorkspace(r)

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
			ws.OwnerType, "connection", conn.ID,
			access.PermQueryExecute,
		)
		if !allowed {
			app.notPermitted(w, r)
			return
		}

		rs, err := session.Query(r.Context(), input.SQL)
		if err != nil {
			app.errorMessage(w, r, http.StatusUnprocessableEntity, err.Error(), nil)
			return
		}

		err = response.JSON(w, http.StatusOK, rs)
		if err != nil {
			app.serverError(w, r, err)
		}
	} else {
		allowed := app.enforcer.Can(r.Context(),
			account.ID, org.ID,
			ws.OwnerType, "connection", conn.ID,
			access.PermConnExecute,
		)
		if !allowed {
			app.notPermitted(w, r)
			return
		}

		rs, err := session.Execute(r.Context(), input.SQL)
		if err != nil {
			app.errorMessage(w, r, http.StatusUnprocessableEntity, err.Error(), nil)
			return
		}

		err = response.JSON(w, http.StatusOK, rs)
		if err != nil {
			app.serverError(w, r, err)
		}
	}
}
