package web

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/catalog"
	"github.com/sqlwarden/internal/credentials"
	"github.com/sqlwarden/internal/execution"
	queryapp "github.com/sqlwarden/internal/query"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
	"github.com/sqlwarden/pkg/result"
)

func (app *application) listConnections(w http.ResponseWriter, r *http.Request) {
	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"name":       "name",
		"created_at": "created_at",
		"driver":     "driver",
	})
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}

	ids := contextGetPathIDs(r)
	in := catalog.ListConnectionsInput{
		OrgID:              ids.OrgID,
		WorkspaceID:        ids.WorkspaceID,
		RouteEnvironmentID: routeEnvironmentID(r),
		Search:             q.Search,
		Driver:             strings.TrimSpace(r.URL.Query().Get("driver")),
		AccessMode:         strings.TrimSpace(r.URL.Query().Get("access_mode")),
		Sort:               q.Sort,
		Order:              q.Order,
		Page:               q.Page,
		PageSize:           q.PageSize,
	}
	if in.AccessMode != "" && in.AccessMode != "open" && in.AccessMode != "restricted" {
		app.failedValidation(w, r, fieldErrors(map[string]string{"access_mode": "Access mode must be open or restricted."}))
		return
	}
	if in.RouteEnvironmentID == nil {
		if rawEnvID := strings.TrimSpace(r.URL.Query().Get("environment_id")); rawEnvID != "" {
			envID, err := strconv.ParseInt(rawEnvID, 10, 64)
			if err != nil || envID < 1 {
				app.failedValidation(w, r, fieldErrors(map[string]string{"environment_id": "Environment must be a positive integer."}))
				return
			}
			in.EnvironmentID = &envID
		}
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	page, err := app.services.Catalog.ListConnections(r.Context(), principal, in)
	if err != nil {
		app.catalogError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, page); err != nil {
		app.serverError(w, r, err)
	}
}

type tlsConfigInput = catalog.TLSConfig
type sshConfigInput = catalog.SSHConfig

func connectionRef(r *http.Request) catalog.ConnRef {
	ids := contextGetPathIDs(r)
	return catalog.ConnRef{
		OrgID:         ids.OrgID,
		WorkspaceID:   ids.WorkspaceID,
		EnvironmentID: routeEnvironmentID(r),
		ConnectionID:  ids.ConnectionID,
	}
}

func routeEnvironmentID(r *http.Request) *int64 {
	if id := contextGetPathIDs(r).EnvironmentID; id != 0 {
		return &id
	}
	return nil
}

func (app *application) requestPrincipal(w http.ResponseWriter, r *http.Request) (access.Principal, bool) {
	principal, ok := contextGetPrincipal(r)
	if !ok {
		app.notPermitted(w, r)
		return access.Principal{}, false
	}
	return principal, true
}

// catalogError translates catalog failures to the standard error envelope.
// Messages never include request values.
func (app *application) catalogError(w http.ResponseWriter, r *http.Request, err error) {
	var validation *catalog.ValidationError
	var reveal *catalog.ErrReveal
	switch {
	case errors.As(err, &validation):
		v := validator.Validator{}
		for field, message := range validation.Fields {
			v.AddFieldError(field, message)
		}
		app.failedValidation(w, r, v)
	case errors.Is(err, catalog.ErrNotFound):
		app.notFound(w, r)
	case errors.Is(err, catalog.ErrForbidden):
		app.notPermitted(w, r)
	case errors.Is(err, catalog.ErrEnvironmentHasConnections):
		app.errorMessage(w, r, http.StatusUnprocessableEntity, "Environment has connections.", nil)
	case errors.Is(err, catalog.ErrActiveSessions):
		app.errorMessage(w, r, http.StatusConflict, "Connection has active sessions. Retry with force=true to apply this change and drop them.", nil)
	case errors.As(err, &reveal):
		app.revealError(w, r, reveal)
	case isTargetPolicyDenial(err):
		if isSQLiteTargetDisabled(err) {
			app.logWarn(r, "sqlite target connection blocked")
		}
		v := validator.Validator{}
		v.AddFieldError("driver", targetConnectionFieldError(err))
		app.failedValidation(w, r, v)
	default:
		app.serverError(w, r, err)
	}
}

func (app *application) revealError(w http.ResponseWriter, r *http.Request, reveal *catalog.ErrReveal) {
	switch reveal.Code {
	case catalog.RevealCodeDisabled:
		app.logWarn(r, "connection secret reveal denied", slog.String("reason", reveal.Code))
		app.apiError(w, r, http.StatusForbidden, catalog.RevealCodeDisabled, "Revealing connection secrets is disabled for this organization.", response.APIError{}, nil)
	default:
		app.apiError(w, r, http.StatusConflict, catalog.RevealCodeNotRevealable, "This secret cannot be revealed.", response.APIError{}, nil)
	}
}

func (app *application) createConnection(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name              string                             `json:"name"`
		Driver            string                             `json:"driver"`
		EnvironmentID     *int64                             `json:"environment_id"`
		Params            catalog.Params                     `json:"params"`
		TLSConfig         *tlsConfigInput                    `json:"tls_config"`
		SSHConfig         *sshConfigInput                    `json:"ssh_config"`
		Secrets           map[credentials.SecretName]*string `json:"secrets"`
		AccessMode        string                             `json:"access_mode"`
		DefaultScope      catalog.ScopePath                  `json:"default_scope,omitempty"`
		ShowSystemSchemas bool                               `json:"show_system_schemas"`
		ShowAllDatabases  bool                               `json:"show_all_databases"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	routeEnv := routeEnvironmentID(r)
	targetEnv := input.EnvironmentID
	if routeEnv != nil {
		targetEnv = routeEnv
	}
	view, err := app.catalogService().Create(r.Context(), principal, catalog.CreateInput{
		OrgID:                      contextGetPathIDs(r).OrgID,
		WorkspaceID:                contextGetPathIDs(r).WorkspaceID,
		EnvironmentID:              targetEnv,
		AuthorizationEnvironmentID: routeEnv,
		Name:                       input.Name,
		Driver:                     input.Driver,
		Params:                     input.Params,
		TLSConfig:                  input.TLSConfig,
		SSHConfig:                  input.SSHConfig,
		Secrets:                    input.Secrets,
		AccessMode:                 input.AccessMode,
		DefaultScope:               input.DefaultScope,
		ShowSystemSchemas:          input.ShowSystemSchemas,
		ShowAllDatabases:           input.ShowAllDatabases,
	})
	if err != nil {
		app.catalogError(w, r, err)
		return
	}
	app.logInfo(r, "connection created", slog.Int64("workspace_id", view.WorkspaceID), slog.Int64("connection_id", view.ID), slog.String("driver", view.Driver), slog.String("access_mode", view.AccessMode))
	if err := response.JSON(w, http.StatusCreated, view); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getConnection(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	view, err := app.catalogService().Get(r.Context(), principal, connectionRef(r))
	if errors.Is(err, catalog.ErrForbidden) {
		app.notFound(w, r)
		return
	}
	if err != nil {
		app.catalogError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, view); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) updateConnection(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name                 *string                            `json:"name"`
		Driver               *string                            `json:"driver"`
		Params               catalog.Params                     `json:"params"`
		TLSConfig            *tlsConfigInput                    `json:"tls_config"`
		SSHConfig            *sshConfigInput                    `json:"ssh_config"`
		Secrets              map[credentials.SecretName]*string `json:"secrets"`
		AccessMode           *string                            `json:"access_mode"`
		SchemaSnapshotPolicy *string                            `json:"schema_snapshot_policy"`
		DefaultScope         *catalog.ScopePath                 `json:"default_scope"`
		ShowSystemSchemas    *bool                              `json:"show_system_schemas"`
		ShowAllDatabases     *bool                              `json:"show_all_databases"`
		Force                bool                               `json:"force"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	if input.Driver != nil {
		app.failedDuplicateField(w, r, "driver", "Driver cannot be changed.")
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	ref := connectionRef(r)
	err := app.catalogService().Update(r.Context(), principal, ref, catalog.UpdateInput{
		Name:                 input.Name,
		Params:               input.Params,
		TLSConfig:            input.TLSConfig,
		SSHConfig:            input.SSHConfig,
		Secrets:              input.Secrets,
		AccessMode:           input.AccessMode,
		SchemaSnapshotPolicy: input.SchemaSnapshotPolicy,
		DefaultScope:         input.DefaultScope,
		ShowSystemSchemas:    input.ShowSystemSchemas,
		ShowAllDatabases:     input.ShowAllDatabases,
		Force:                input.Force,
	})
	if err != nil {
		app.catalogError(w, r, err)
		return
	}
	app.logInfo(r, "connection updated", slog.Int64("connection_id", ref.ConnectionID))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) deleteConnection(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	ref := connectionRef(r)
	if err := app.catalogService().Delete(r.Context(), principal, ref); err != nil {
		app.catalogError(w, r, err)
		return
	}
	app.logInfo(r, "connection deleted", slog.Int64("connection_id", ref.ConnectionID), slog.Int64("workspace_id", ref.WorkspaceID))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) testConnection(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ConnectionID *int64                             `json:"connection_id"`
		Driver       string                             `json:"driver"`
		Params       catalog.Params                     `json:"params"`
		TLSConfig    *tlsConfigInput                    `json:"tls_config"`
		SSHConfig    *sshConfigInput                    `json:"ssh_config"`
		Secrets      map[credentials.SecretName]*string `json:"secrets"`
		ParentScope  catalog.ScopePath                  `json:"parent_scope,omitempty"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	settings, err := app.services.Settings.EffectiveForOrg(r.Context(), nil)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	routeEnv := routeEnvironmentID(r)
	result, err := app.catalogService().Test(r.Context(), principal, catalog.TestInput{
		OrgID:                      contextGetPathIDs(r).OrgID,
		WorkspaceID:                contextGetPathIDs(r).WorkspaceID,
		EnvironmentID:              routeEnv,
		AuthorizationEnvironmentID: routeEnv,
		ConnectionID:               input.ConnectionID,
		Driver:                     input.Driver,
		Params:                     input.Params,
		TLSConfig:                  input.TLSConfig,
		SSHConfig:                  input.SSHConfig,
		Secrets:                    input.Secrets,
		ParentScope:                input.ParentScope,
		Limits:                     queryLimits(settings),
	})
	if err != nil {
		app.catalogError(w, r, err)
		return
	}
	if !result.OK {
		app.logWarn(r, "connection test failed", slog.String("driver", input.Driver), slog.Int64("latency_ms", result.LatencyMS), slog.String("stage", result.Stage), slog.String("error_category", result.ErrorCategory))
		switch result.Stage {
		case catalog.TestStageSSHTunnel:
			app.errorMessage(w, r, http.StatusUnprocessableEntity, result.Error, nil)
			return
		case catalog.TestStageDriverInit:
			if err := response.JSON(w, http.StatusUnprocessableEntity, map[string]any{"ok": false, "error": result.Error}); err != nil {
				app.serverError(w, r, err)
			}
			return
		}
	} else {
		app.logInfo(r, "connection test completed", slog.String("driver", input.Driver), slog.Int64("latency_ms", result.LatencyMS), slog.Bool("ok", true))
	}
	if err := response.JSON(w, http.StatusOK, result); err != nil {
		app.serverError(w, r, err)
	}
}

// revealNoStore marks reveal responses uncacheable before connection
// resolution runs, so errors raised by that middleware carry the header too.
func revealNoStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/reveal") {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func (app *application) revealConnectionSecret(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	value, err := app.catalogService().RevealSecret(r.Context(), principal, connectionRef(r), credentials.SecretName(chi.URLParam(r, "name")))
	if errors.Is(err, catalog.ErrForbidden) {
		app.logWarn(r, "connection secret reveal denied", slog.String("reason", "forbidden"))
	}
	if err != nil {
		app.catalogError(w, r, err)
		return
	}
	err = response.JSON(w, http.StatusOK, map[string]string{"value": value})
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) connectToDatabase(w http.ResponseWriter, r *http.Request) {
	ids := contextGetPathIDs(r)

	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	session, err := app.queryService().Connect(r.Context(), principal, connectionRef(r))
	if err != nil {
		if errors.Is(err, access.ErrForbidden) || errors.Is(err, access.ErrNotFound) {
			app.serviceError(w, r, err)
			return
		}
		if isTargetPolicyDenial(err) {
			if isSQLiteTargetDisabled(err) {
				app.logWarn(r, "sqlite target connection blocked", slog.String("operation", "connect"), slog.Int64("connection_id", ids.ConnectionID))
			}
			app.errorMessage(w, r, http.StatusUnprocessableEntity, targetConnectionFieldError(err), nil)
			return
		}
		var (
			tunnelErr  *execution.TunnelError
			connectErr *execution.ConnectError
		)
		if errors.As(err, &tunnelErr) || errors.As(err, &connectErr) {
			app.logWarn(r, "database session open failed", slog.Int64("connection_id", ids.ConnectionID), slog.String("error_category", connectionTestErrorCategory(err)))
			message := "Connection failed."
			resolved, resolveErr := app.credentials.Resolve(r.Context(), credentials.ConnectionRef{
				OrgID:        strconv.FormatInt(ids.OrgID, 10),
				WorkspaceID:  strconv.FormatInt(ids.WorkspaceID, 10),
				ConnectionID: strconv.FormatInt(ids.ConnectionID, 10),
			})
			if resolveErr == nil {
				message = catalog.RedactConnectionError(err.Error(), resolved)
			}
			app.errorMessage(w, r, http.StatusUnprocessableEntity, message, nil)
			return
		}
		app.serverError(w, r, err)
		return
	}

	app.logInfo(r, "database session opened", slog.Int64("connection_id", ids.ConnectionID), slog.String("session_id", string(session.ID)), slog.Bool("reused", session.Reused))
	err = response.JSON(w, http.StatusOK, map[string]any{
		"session_id": session.ID,
		"reused":     session.Reused,
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
	case isTargetPolicyDenial(err):
		return "policy_denied"
	case strings.Contains(err.Error(), "unknown driver"):
		return "unsupported_driver"
	default:
		return "target_unreachable"
	}
}

func (app *application) listActiveSessions(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	sessions, err := app.queryService().ListWorkspaceSessions(r.Context(), principal, contextGetPathIDs(r).OrgID, contextGetPathIDs(r).WorkspaceID)
	if err != nil {
		app.queryError(w, r, err)
		return
	}

	type sessionInfo struct {
		ConnectionID  int64  `json:"connection_id"`
		AccountID     int64  `json:"account_id"`
		SessionID     string `json:"session_id"`
		TunnelHealthy *bool  `json:"tunnel_healthy,omitempty"`
	}
	result := make([]sessionInfo, 0, len(sessions))
	for _, session := range sessions {
		connIDInt, parseErr := strconv.ParseInt(session.Scope.ConnectionID, 10, 64)
		if parseErr != nil {
			continue
		}
		accountIDInt, parseErr := strconv.ParseInt(session.Scope.AccountID, 10, 64)
		if parseErr != nil {
			continue
		}
		result = append(result, sessionInfo{
			ConnectionID:  connIDInt,
			AccountID:     accountIDInt,
			SessionID:     string(session.ID),
			TunnelHealthy: session.TunnelHealthy,
		})
	}

	err = response.JSON(w, http.StatusOK, map[string]any{"sessions": result})
	if err != nil {
		app.serverError(w, r, err)
	}
}

// disconnectFromDatabase is idempotent: a session that is already gone or
// belongs to another scope answers 204 without being touched.
func (app *application) disconnectFromDatabase(w http.ResponseWriter, r *http.Request) {
	sessionID := requestSessionID(r)
	if sessionID == "" {
		app.badRequest(w, r, errors.New("X-Warden-Session header is required"))
		return
	}

	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	if err := app.queryService().Disconnect(r.Context(), principal, connectionRef(r), sessionID); err != nil {
		app.queryError(w, r, err)
		return
	}
	app.logInfo(r, "database session disconnected", slog.Int64("connection_id", contextGetPathIDs(r).ConnectionID), slog.String("session_id", string(sessionID)))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) revokeWorkspaceDatabaseSession(w http.ResponseWriter, r *http.Request) {
	ids := contextGetPathIDs(r)
	sessionID := strings.TrimSpace(chi.URLParam(r, "session_id"))
	if sessionID == "" {
		app.notFound(w, r)
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	session, err := app.queryService().RevokeSession(r.Context(), principal, ids.OrgID, ids.WorkspaceID, sessionID)
	if err != nil {
		app.queryError(w, r, err)
		return
	}
	if session == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	app.logInfo(r, "database session revoked", slog.Int64("workspace_id", ids.WorkspaceID), slog.String("session_id", sessionID), slog.String("session_account_id", session.Scope.AccountID))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) executeQuery(w http.ResponseWriter, r *http.Request) {
	var input struct {
		SQL           string `json:"sql"`
		Explain       string `json:"explain,omitempty"`
		PageSize      *int   `json:"page_size"`
		UseCursor     *bool  `json:"use_cursor"`
		ConfirmUnsafe bool   `json:"confirm_unsafe"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	out, err := app.queryService().Execute(r.Context(), principal, connectionRef(r), queryapp.ExecuteInput{
		SessionID:     requestSessionID(r),
		SQL:           input.SQL,
		Explain:       input.Explain,
		PageSize:      input.PageSize,
		UseCursor:     input.UseCursor,
		ConfirmUnsafe: input.ConfirmUnsafe,
	})
	if err != nil {
		app.queryRuntimeError(w, r, err)
		return
	}
	err = response.JSON(w, http.StatusOK, struct {
		*result.ResultSet
		Transaction transactionStatusView `json:"transaction"`
	}{ResultSet: out.Result, Transaction: newTransactionStatusView(out.Transaction)})
	if err != nil {
		app.serverError(w, r, err)
	}
}
