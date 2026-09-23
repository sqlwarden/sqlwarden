package web

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/catalog"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/classifier"
	"github.com/sqlwarden/internal/engine/explain"
	metadata "github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/engine/safety"
	"github.com/sqlwarden/internal/execution"
	"github.com/sqlwarden/internal/jobs"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	settingsapp "github.com/sqlwarden/internal/settings"
	"github.com/sqlwarden/internal/validator"
	"github.com/sqlwarden/pkg/result"
)

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

	query := catalog.ListConnectionsQuery{
		ListQuery: catalog.ListQuery{
			Search: q.Search, Sort: q.Sort, Order: q.Order, Page: q.Page, PageSize: q.PageSize,
		},
		Driver:     strings.TrimSpace(r.URL.Query().Get("driver")),
		AccessMode: strings.TrimSpace(r.URL.Query().Get("access_mode")),
	}
	if query.AccessMode != "" && query.AccessMode != catalog.AccessModeOpen && query.AccessMode != catalog.AccessModeRestricted {
		app.failedValidation(w, r, fieldErrors(map[string]string{"access_mode": "Access mode must be open or restricted."}))
		return
	}
	if env.ID != 0 {
		query.EnvironmentID = &env.ID
	} else if rawEnvID := strings.TrimSpace(r.URL.Query().Get("environment_id")); rawEnvID != "" {
		envID, err := strconv.ParseInt(rawEnvID, 10, 64)
		if err != nil || envID < 1 {
			app.failedValidation(w, r, fieldErrors(map[string]string{"environment_id": "Environment must be a positive integer."}))
			return
		}
		query.EnvironmentID = &envID
	}
	account := contextGetAccount(r)
	result, err := app.catalogService().ListConnections(r.Context(), account.ID, org.ID, ws.ID, query)
	if err != nil {
		app.catalogError(w, r, err)
		return
	}

	err = response.JSON(w, http.StatusOK, result)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func queryLogAttrs(account database.Account, org database.Organization, ws database.Workspace, conn database.Connection, classification classifier.Result) []slog.Attr {
	return []slog.Attr{
		slog.Group("account", "id", account.ID),
		slog.Group("org", "id", org.ID, "slug", org.Slug),
		slog.Group("workspace", "id", ws.ID, "owner_type", ws.OwnerType),
		slog.Group("connection", "id", conn.ID, "driver", conn.Driver),
		slog.Group("query", "kind", classification.Kind, "classifier", classification.Source),
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
	return app.policyEvaluator.Can(r.Context(), account.ID, orgID, ownerType, "connection", connectionID, permission)
}

func (app *application) classifyConnectionSQL(r *http.Request, conn database.Connection, sql string) (classifier.Result, error) {
	return connectionClassifier(conn.Driver).Classify(r.Context(), classifier.Request{SQL: sql})
}

// registeredConnectionClassifier resolves only a classifier implemented by the
// registered engine. Callers that must prove SQL properties, such as exports,
// must not fall back to a heuristic.
func registeredConnectionClassifier(driverName string) (classifier.Classifier, bool) {
	d, err := engine.New(driverName)
	if err != nil {
		return nil, false
	}
	c, ok := d.(classifier.Classifier)
	return c, ok
}

// connectionClassifier resolves a stateless classifier for a connection's
// driver by type-asserting a fresh (unconnected) driver instance — the same
// pattern as schema/cursor capabilities — and falls back to the conservative
// heuristic when the driver does not implement classification.
func connectionClassifier(driverName string) classifier.Classifier {
	if c, ok := registeredConnectionClassifier(driverName); ok {
		return c
	}
	return classifier.NewHeuristic()
}

func (app *application) checkConnectionSQLSafety(r *http.Request, conn database.Connection, sql string) (safety.Result, error) {
	return connectionSafetyChecker(conn.Driver).Check(r.Context(), safety.Request{SQL: sql})
}

// registeredConnectionSafetyChecker resolves only a checker implemented by
// the registered engine, mirroring registeredConnectionClassifier.
func registeredConnectionSafetyChecker(driverName string) (safety.Checker, bool) {
	d, err := engine.New(driverName)
	if err != nil {
		return nil, false
	}
	c, ok := d.(safety.Checker)
	return c, ok
}

// connectionSafetyChecker resolves a stateless safety checker for a
// connection's driver, falling back to the conservative heuristic when the
// driver does not implement Checker — the same fallback shape as
// connectionClassifier.
func connectionSafetyChecker(driverName string) safety.Checker {
	if c, ok := registeredConnectionSafetyChecker(driverName); ok {
		return c
	}
	return safety.NewHeuristic()
}

// registeredConnectionExplainer resolves an Explainer implemented by the
// registered engine, mirroring registeredConnectionClassifier. There is no
// heuristic fallback: an engine either has a real EXPLAIN form or it doesn't.
func registeredConnectionExplainer(driverName string) (explain.Explainer, bool) {
	d, err := engine.New(driverName)
	if err != nil {
		return nil, false
	}
	e, ok := d.(explain.Explainer)
	return e, ok
}

func (app *application) createConnection(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name              string              `json:"name"`
		Driver            string              `json:"driver"`
		DSN               string              `json:"dsn"`
		EnvironmentID     *int64              `json:"environment_id"`
		AccessMode        string              `json:"access_mode"`
		DefaultScope      metadata.ScopePath  `json:"default_scope,omitempty"`
		ShowSystemSchemas bool                `json:"show_system_schemas"`
		TLS               *tlsConfigDocument  `json:"tls"`
		SSH               *sshConfigDocument  `json:"ssh"`
		V                 validator.Validator `json:"-"`
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
		DefaultScope: input.DefaultScope, ShowSystemSchemas: input.ShowSystemSchemas,
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

	app.logInfo(r, "connection created", slog.Int64("workspace_id", ws.ID), slog.Int64("connection_id", conn.ID), slog.String("driver", conn.Driver), slog.String("access_mode", conn.AccessMode))
	err = response.JSON(w, http.StatusCreated, conn)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getConnection(w http.ResponseWriter, r *http.Request) {
	conn := contextGetConnection(r)
	ws := contextGetWorkspace(r)
	resolved, err := app.catalogService().Connection(r.Context(), contextGetAccount(r).ID, contextGetOrg(r).ID, ws, conn)
	if err != nil {
		app.catalogError(w, r, err)
		return
	}
	err = response.JSON(w, http.StatusOK, resolved)
	if err != nil {
		app.serverError(w, r, err)
	}
}

// getConnectionDSN reveals the decrypted DSN so it can be pre-filled when editing a
// connection. The route requires conn:update, since holding conn:update is what makes
// re-entering the DSN unnecessary.
func (app *application) getConnectionDSN(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	conn := contextGetConnection(r)
	dsn, err := app.catalogService().RevealConnectionDSN(r.Context(), catalogActor(r), org, conn)
	if err != nil {
		app.catalogError(w, r, err)
		return
	}
	app.logInfo(r, "connection dsn revealed", slog.Int64("connection_id", conn.ID))
	err = response.JSON(w, http.StatusOK, map[string]string{"dsn": dsn})
	if err != nil {
		app.serverError(w, r, err)
	}
}

// getConnectionTLS reveals the stored TLS config, minus the private key, so the
// edit form can pre-fill it. Gated by conn:update like getConnectionDSN.
func (app *application) getConnectionTLS(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	if org.MaskConnectionCredentialsOnEdit {
		app.notPermitted(w, r)
		return
	}

	conn := contextGetConnection(r)
	doc, has, err := app.decodeTLSDocument(conn.TLSConfigEncrypted)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	mode := doc.Mode
	if !has || mode == "" {
		mode = string(engine.TLSModeDisable)
	}
	app.logInfo(r, "connection tls revealed", slog.Int64("connection_id", conn.ID))
	err = response.JSON(w, http.StatusOK, map[string]any{
		"configured":      has,
		"mode":            mode,
		"server_name":     doc.ServerName,
		"ca_pem":          doc.CAPEM,
		"client_cert_pem": doc.ClientCertPEM,
		"client_key_set":  doc.ClientKeyPEM != "",
	})
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) updateConnection(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name                 *string             `json:"name"`
		Driver               *string             `json:"driver"`
		DSN                  *string             `json:"dsn"`
		AccessMode           *string             `json:"access_mode"`
		SchemaSnapshotPolicy *string             `json:"schema_snapshot_policy"`
		DefaultScope         *metadata.ScopePath `json:"default_scope"`
		ShowSystemSchemas    *bool               `json:"show_system_schemas"`
		TLS                  *tlsConfigDocument  `json:"tls"`
		SSH                  *sshConfigDocument  `json:"ssh"`
		Force                bool                `json:"force"`
		V                    validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.V.CheckField(input.Driver == nil, "driver", "Driver cannot be changed.")
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		input.Name = &name
		input.V.CheckField(name != "", "name", "Name must not be empty.")
	}
	if input.DSN != nil {
		input.V.CheckField(strings.TrimSpace(*input.DSN) != "", "dsn", "DSN must not be empty.")
	}
	if input.AccessMode != nil {
		input.V.CheckField(*input.AccessMode == "open" || *input.AccessMode == "restricted",
			"access_mode", "Access mode must be open or restricted.")
	}
	if input.SchemaSnapshotPolicy != nil {
		input.V.CheckField(*input.SchemaSnapshotPolicy == database.SchemaSnapshotPolicyInherit ||
			*input.SchemaSnapshotPolicy == database.SchemaSnapshotPolicyDisabled,
			"schema_snapshot_policy", "Schema snapshot policy must be inherit or disabled.")
	}
	input.V.CheckField(input.Name != nil || input.DSN != nil || input.AccessMode != nil || input.SchemaSnapshotPolicy != nil || input.DefaultScope != nil || input.ShowSystemSchemas != nil || input.TLS != nil || input.SSH != nil,
		"request", "At least one setting is required.")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	conn := contextGetConnection(r)

	tlsEncrypted := ""
	tlsChanged := false
	if input.TLS != nil {
		next := *input.TLS
		clearClientKey := next.ClearClientKey
		next.ClearClientKey = false
		current, hasCurrent, decodeErr := app.decodeTLSDocument(conn.TLSConfigEncrypted)
		if decodeErr != nil {
			app.serverError(w, r, decodeErr)
			return
		}
		if next.ClientKeyPEM == "" && hasCurrent && !clearClientKey {
			next.ClientKeyPEM = current.ClientKeyPEM
		}
		tlsV := validator.Validator{}
		app.validateTLSDocument(conn.Driver, next, &tlsV)
		if tlsV.HasErrors() {
			app.failedValidation(w, r, tlsV)
			return
		}
		tlsEncrypted, err = app.sealTLSDocument(next)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		tlsChanged = true
	}

	sshEncrypted := ""
	sshChanged := false
	if input.SSH != nil {
		next := *input.SSH
		clearPassword := next.ClearPassword
		clearPrivateKey := next.ClearPrivateKey
		clearPassphrase := next.ClearPassphrase
		next.ClearPassword, next.ClearPrivateKey, next.ClearPassphrase = false, false, false
		current, hasCurrent, decodeErr := app.decodeSSHDocument(conn.SSHConfigEncrypted)
		if decodeErr != nil {
			app.serverError(w, r, decodeErr)
			return
		}
		if hasCurrent {
			if next.Password == "" && !clearPassword {
				next.Password = current.Password
			}
			if next.PrivateKeyPEM == "" && !clearPrivateKey {
				next.PrivateKeyPEM = current.PrivateKeyPEM
			}
			if next.Passphrase == "" && !clearPassphrase {
				next.Passphrase = current.Passphrase
			}
		}
		sshV := validator.Validator{}
		app.validateSSHDocument(conn.Driver, next, &sshV)
		if sshV.HasErrors() {
			app.failedValidation(w, r, sshV)
			return
		}
		sshEncrypted, err = app.sealSSHDocument(next)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		sshChanged = true
	}

	var sealedTLS, sealedSSH *string
	if tlsChanged {
		sealedTLS = &tlsEncrypted
	}
	if sshChanged {
		sealedSSH = &sshEncrypted
	}
	result, err := app.catalogService().UpdateConnection(r.Context(), catalogActor(r), conn, catalog.UpdateConnectionInput{
		Name: input.Name, DSN: input.DSN, AccessMode: input.AccessMode,
		SchemaSnapshotPolicy: input.SchemaSnapshotPolicy, DefaultScope: input.DefaultScope,
		ShowSystemSchemas: input.ShowSystemSchemas, SealedTLS: sealedTLS, SealedSSH: sealedSSH,
		Force: input.Force,
	})
	if err != nil {
		app.catalogError(w, r, err)
		return
	}
	if result.DSNRotated && result.DroppedSessions > 0 {
		app.logInfo(r, "connection sessions dropped for dsn rotation", slog.Int64("connection_id", conn.ID), slog.Int("dropped_sessions", result.DroppedSessions))
	}
	if result.SnapshotsDisabled {
		if err := app.disableConnectionSnapshots(r.Context(), conn.ID); err != nil {
			app.serverError(w, r, err)
			return
		}
	}
	if result.ScopeChanged {
		app.schemaService.RefreshConnection(strconv.FormatInt(conn.ID, 10))
		app.completionService.InvalidateConnection(strconv.FormatInt(conn.ID, 10))
		if snapshotsEnabled, enabledErr := app.db.SchemaSnapshotsEnabled(r.Context(), conn.ID); enabledErr != nil {
			app.logWarn(r, "schema snapshot policy lookup failed after scope change",
				slog.Int64("connection_id", conn.ID), slog.String("error", enabledErr.Error()))
		} else if snapshotsEnabled {
			if _, _, enqueueErr := app.enqueueSchemaSync(r.Context(), conn.ID, contextGetWorkspace(r).OrgID); enqueueErr != nil &&
				!errors.Is(enqueueErr, jobs.ErrActiveExists) {
				app.logWarn(r, "schema sync enqueue failed after scope change",
					slog.Int64("connection_id", conn.ID), slog.String("error", enqueueErr.Error()))
			}
		}
	}
	if tlsChanged {
		app.logInfo(r, "connection tls updated", slog.Int64("connection_id", conn.ID))
	}
	if sshChanged {
		app.logInfo(r, "connection ssh updated", slog.Int64("connection_id", conn.ID))
	}

	app.logInfo(r, "connection updated", slog.Int64("connection_id", conn.ID), slog.Bool("dsn_rotated", result.DSNRotated), slog.Bool("scope_changed", result.ScopeChanged), slog.String("access_mode", result.Connection.AccessMode), slog.String("schema_snapshot_policy", result.Connection.SchemaSnapshotPolicy))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) deleteConnection(w http.ResponseWriter, r *http.Request) {
	conn := contextGetConnection(r)
	err := app.catalogService().DeleteConnection(r.Context(), catalogActor(r), conn)
	if err != nil {
		app.catalogError(w, r, err)
		return
	}
	app.logInfo(r, "connection deleted", slog.Int64("connection_id", conn.ID), slog.Int64("workspace_id", conn.WorkspaceID), slog.String("driver", conn.Driver))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) testConnection(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Driver      string              `json:"driver"`
		DSN         string              `json:"dsn"`
		ParentScope metadata.ScopePath  `json:"parent_scope,omitempty"`
		TLS         *tlsConfigDocument  `json:"tls"`
		SSH         *sshConfigDocument  `json:"ssh"`
		V           validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.V.CheckField(input.Driver != "", "driver", "Driver is required.")
	input.V.CheckField(input.DSN != "", "dsn", "DSN is required.")
	var tlsCfg *engine.TLSConfig
	if input.TLS != nil {
		app.validateTLSDocument(input.Driver, *input.TLS, &input.V)
		tlsCfg = input.TLS.toEngine()
	}
	var sshCfg *execution.SSHConfig
	if input.SSH != nil {
		app.validateSSHDocument(input.Driver, *input.SSH, &input.V)
		sshCfg = input.SSH.toConnection()
	}
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}
	if err := app.validateTargetConnection(r.Context(), input.Driver, input.DSN); err != nil {
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

	d, err := engine.New(input.Driver)
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

	settings, err := app.settingsService().EffectiveForOrg(ctx, nil)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	var tunnel *execution.SSHTunnel
	if sshCfg != nil {
		tunnel, err = execution.OpenSSHTunnel(ctx, *sshCfg)
		if err != nil {
			latency := time.Since(start).Milliseconds()
			app.logWarn(r, "connection test failed", slog.String("driver", input.Driver), slog.Int64("latency_ms", latency), slog.String("stage", "ssh_tunnel"), slog.String("error_category", connectionTestErrorCategory(err)))
			app.errorMessage(w, r, http.StatusUnprocessableEntity, "SSH tunnel: "+err.Error(), nil)
			return
		}
		defer tunnel.Close()
	}
	cc := app.driverConnectionConfig(input.Driver, input.DSN, settings, tlsCfg)
	if tunnel != nil {
		cc.SSHDialer = tunnel.DialContext
	}
	err = d.Connect(ctx, cc)
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

	payload := map[string]any{
		"ok":         true,
		"latency_ms": latency,
	}
	if discoverer, ok := d.(metadata.ScopeDiscoverer); ok {
		discovery, discoveryErr := discoverer.DiscoverScopes(ctx, metadata.ScopeDiscoveryRequest{Parent: input.ParentScope})
		if discoveryErr == nil {
			payload["scope_discovery"] = discovery
		} else {
			payload["scope_discovery_error"] = discoveryErr.Error()
		}
	}
	app.logInfo(r, "connection test completed", slog.String("driver", input.Driver), slog.Int64("latency_ms", latency), slog.Bool("ok", true))
	err = response.JSON(w, http.StatusOK, payload)
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

	settings, err := app.settingsService().EffectiveForWorkspace(r.Context(), ws)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	openCtx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	opened, err := app.executionRuntime.Open(openCtx, execution.OpenRequest{
		Scope:  execution.ParseNumericScope(account.ID, org.ID, ws.ID, conn.ID),
		Limits: execution.Limits{MaxRows: settings.QueryMaxResultRows, MaxBytes: settings.QueryMaxResultBytes},
	})
	if err != nil {
		switch {
		case isTargetRejected(err):
			app.errorMessage(w, r, http.StatusUnprocessableEntity, targetConnectionFieldError(err), nil)
		case errors.Is(err, execution.ErrTargetConnection):
			app.errorMessage(w, r, http.StatusUnprocessableEntity, execution.ErrTargetConnection.Error(), nil)
		default:
			app.serverError(w, r, err)
		}
		return
	}

	app.logInfo(r, "database session opened", slog.Int64("connection_id", conn.ID), slog.Bool("reused", opened.Reused))
	app.maybeEnqueueSchemaSync(context.WithoutCancel(r.Context()), conn, ws.OrgID)
	err = response.JSON(w, http.StatusOK, map[string]any{
		"session_id": string(opened.Handle),
		"reused":     opened.Reused,
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
		ConnectionID  int64  `json:"connection_id"`
		AccountID     int64  `json:"account_id"`
		SessionID     string `json:"session_id"`
		TunnelHealthy *bool  `json:"tunnel_healthy,omitempty"`
	}
	result := make([]sessionInfo, 0)

	listScope := execution.Scope{AccountID: accountID, WorkspaceID: workspaceID}
	if org.ID != 0 && app.policyEvaluator.Can(r.Context(), account.ID, org.ID, ws.OwnerType, "workspace", ws.ID, access.PermPolicyRead) {
		listScope.AccountID = ""
	}
	refs, err := app.executionRuntime.Sessions(r.Context(), listScope)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	for _, ref := range refs {
		if ref.Scope.WorkspaceID != "" && ref.Scope.WorkspaceID != workspaceID {
			continue
		}
		connIDInt, parseErr := strconv.ParseInt(ref.Scope.ConnectionID, 10, 64)
		if parseErr != nil {
			continue
		}
		accountIDInt, parseErr := strconv.ParseInt(ref.Scope.AccountID, 10, 64)
		if parseErr != nil {
			continue
		}
		result = append(result, sessionInfo{
			ConnectionID:  connIDInt,
			AccountID:     accountIDInt,
			SessionID:     string(ref.Handle),
			TunnelHealthy: ref.TunnelHealthy,
		})
	}

	err = response.JSON(w, http.StatusOK, map[string]any{"sessions": result})
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

	handle := execution.SessionHandle(sessionID)
	session, found, err := app.executionRuntime.Session(r.Context(), handle)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		// Session already gone (expired or never existed) — idempotent.
		w.WriteHeader(http.StatusNoContent)
		return
	}

	accountID := strconv.FormatInt(account.ID, 10)
	if session.Scope.AccountID != accountID {
		app.notPermitted(w, r)
		return
	}

	connID := strconv.FormatInt(conn.ID, 10)
	if session.Scope.ConnectionID != connID {
		app.badRequest(w, r, errors.New("session does not belong to this connection"))
		return
	}

	if err := app.executionRuntime.Close(r.Context(), execution.CloseRequest{Handle: handle}); err != nil {
		app.serverError(w, r, err)
		return
	}
	if app.executionRuntime.CountForConnection(connID) == 0 {
		if persistent, policyErr := app.db.SchemaSnapshotsEnabled(r.Context(), conn.ID); policyErr == nil && !persistent {
			app.schemaService.RefreshConnection(connID)
		}
	}
	app.logInfo(r, "database session disconnected", slog.Int64("connection_id", conn.ID))
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

	handle := execution.SessionHandle(sessionID)
	session, found, err := app.executionRuntime.Session(r.Context(), handle)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	workspaceID := strconv.FormatInt(ws.ID, 10)
	if session.Scope.WorkspaceID != workspaceID {
		app.notFound(w, r)
		return
	}

	accountID := strconv.FormatInt(account.ID, 10)
	if session.Scope.AccountID != accountID {
		if org.ID == 0 || !app.policyEvaluator.Can(r.Context(), account.ID, org.ID, ws.OwnerType, "workspace", ws.ID, access.PermPolicyModify) {
			app.notPermitted(w, r)
			return
		}
	}

	if err := app.executionRuntime.Close(r.Context(), execution.CloseRequest{Handle: handle}); err != nil {
		app.serverError(w, r, err)
		return
	}
	app.logInfo(r, "database session revoked", slog.Int64("workspace_id", ws.ID), slog.String("session_account_id", session.Scope.AccountID))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) driverConnectionConfig(driverName, dsn string, settings settingsapp.Effective, tls *engine.TLSConfig, defaultScopes ...metadata.ScopePath) engine.ConnectionConfig {
	config := engine.ConnectionConfig{
		DSN:            dsn,
		Driver:         driverName,
		MaxResultRows:  settings.QueryMaxResultRows,
		MaxResultBytes: settings.QueryMaxResultBytes,
		TLS:            tls,
	}
	if len(defaultScopes) > 0 {
		config.DefaultScope = defaultScopes[0]
	}
	return config
}

func (app *application) executeQuery(w http.ResponseWriter, r *http.Request) {
	var input struct {
		SQL           string              `json:"sql"`
		Explain       string              `json:"explain,omitempty"`
		PageSize      *int                `json:"page_size"`
		UseCursor     *bool               `json:"use_cursor"`
		ConfirmUnsafe bool                `json:"confirm_unsafe"`
		V             validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.V.CheckField(input.SQL != "", "sql", "SQL is required.")
	input.V.CheckField(
		input.Explain == "" || input.Explain == string(explain.ModePlain) || input.Explain == string(explain.ModeAnalyze),
		"explain", `Explain must be "plain" or "analyze".`,
	)
	if input.PageSize != nil {
		input.V.CheckField(*input.PageSize > 0, "page_size", "Page size must be greater than 0.")
	}
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}
	runtimeSettings, err := app.settingsService().EffectiveForWorkspace(r.Context(), contextGetWorkspace(r))
	if err != nil {
		app.serverError(w, r, err)
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

	handle := execution.SessionHandle(sessionID)
	session, found, err := app.executionRuntime.Session(r.Context(), handle)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.errorMessage(w, r, http.StatusGone, "Session has expired or does not exist.", nil)
		return
	}

	if session.Scope.AccountID != strconv.FormatInt(account.ID, 10) {
		app.notPermitted(w, r)
		return
	}
	if session.Scope.ConnectionID != strconv.FormatInt(conn.ID, 10) {
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
	if classification.Kind == classifier.KindUnknown {
		app.logWarn(r, "query classification unknown", logAttrs...)
	} else {
		app.logDebug(r, "query classified", logAttrs...)
	}

	// execSQL is what actually reaches the target database. Permission and
	// safety decisions above use classification, which is always derived from
	// the caller's original, unwrapped input.SQL — EXPLAIN never changes what
	// permission a statement requires or whether it needs unsafe confirmation.
	execSQL := input.SQL
	explainMode := explain.Mode(input.Explain)
	var explainPlan explain.Plan
	if input.Explain != "" {
		explainer, ok := registeredConnectionExplainer(conn.Driver)
		if !ok {
			app.errorMessage(w, r, http.StatusUnprocessableEntity, "This connection does not support EXPLAIN.", nil)
			return
		}
		if explainMode == explain.ModeAnalyze && !explainer.ExplainSpec().SupportsAnalyze {
			app.errorMessage(w, r, http.StatusUnprocessableEntity, "This connection does not support EXPLAIN ANALYZE.", nil)
			return
		}
		// Explain validates sql itself (statement count, already-EXPLAIN) —
		// callers don't pre-process it.
		plan, explainErr := explainer.Explain(input.SQL, explainMode)
		if explainErr != nil {
			switch {
			case errors.Is(explainErr, explain.ErrMultipleStatements):
				app.logWarn(r, "explain refused for multi-statement input", logAttrs...)
				app.errorMessage(w, r, http.StatusUnprocessableEntity, "EXPLAIN requires exactly one statement.", nil)
			case errors.Is(explainErr, explain.ErrAlreadyExplained):
				app.errorMessage(w, r, http.StatusUnprocessableEntity, "This statement is already an EXPLAIN statement.", nil)
			default:
				app.serverError(w, r, explainErr)
			}
			return
		}
		explainPlan = plan
		execSQL = plan.Statement
	}

	var rs *result.ResultSet
	var execErr error
	start := time.Now()

	// For an EXPLAIN request, explainPlan.Statement is always a query whose
	// result set is the plan output, no matter how the underlying statement
	// classifies. Preparatory statements (Oracle's EXPLAIN PLAN FOR, the
	// ALTER SESSION pair for ANALYZE) run first for their side effects and
	// their result sets are discarded. Setup runs after the per-class
	// permission check below so planning a statement still requires permission
	// to run that class of statement.
	executeExplainPlan := func() (*result.ResultSet, error) {
		queried, err := app.executionRuntime.Query(r.Context(), execution.QueryRequest{
			Handle: handle, Explain: &explainPlan,
			Limits: execution.Limits{MaxRows: runtimeSettings.QueryMaxResultRows, MaxBytes: runtimeSettings.QueryMaxResultBytes},
		})
		if err != nil {
			return nil, err
		}
		rs := queried.Result
		rs.DurationMs = time.Since(start).Milliseconds()
		return rs, nil
	}
	execStatement := func() (*result.ResultSet, error) {
		executed, err := app.executionRuntime.Execute(r.Context(), execution.ExecuteRequest{
			Handle: handle, SQL: execSQL,
			Limits: execution.Limits{MaxRows: runtimeSettings.QueryMaxResultRows, MaxBytes: runtimeSettings.QueryMaxResultBytes},
		})
		return executed.Result, err
	}

	switch classification.Kind {
	case classifier.KindDQL:
		if !hasBroadExecute && !app.policyEvaluator.Can(r.Context(),
			account.ID, org.ID,
			ws.OwnerType, "connection", conn.ID,
			access.PermConnDQL,
		) {
			app.logWarn(r, "query permission denied", append(logAttrs, slog.String("required_permission", access.PermConnDQL))...)
			app.notPermitted(w, r)
			return
		}
		if input.Explain != "" {
			rs, execErr = executeExplainPlan()
		} else {
			rs, execErr = app.executeDQLQuery(r, handle, execSQL, input.UseCursor, input.PageSize, start, runtimeSettings)
		}
	case classifier.KindDML:
		if !hasBroadExecute && !app.policyEvaluator.Can(r.Context(),
			account.ID, org.ID,
			ws.OwnerType, "connection", conn.ID,
			access.PermConnDML,
		) {
			app.logWarn(r, "query permission denied", append(logAttrs, slog.String("required_permission", access.PermConnDML))...)
			app.notPermitted(w, r)
			return
		}
		// Plain EXPLAIN only plans the statement; it never runs it, so the
		// no-WHERE confirmation gate (which exists to stop real mutations)
		// does not apply. EXPLAIN ANALYZE does run it for real and stays gated.
		if !input.ConfirmUnsafe && explainMode != explain.ModePlain {
			safetyResult, safetyErr := app.checkConnectionSQLSafety(r, conn, input.SQL)
			if safetyErr != nil {
				app.serverError(w, r, safetyErr)
				return
			}
			if safetyResult.Unsafe {
				app.logWarn(r, "unsafe query refused pending confirmation", append(logAttrs, slog.Int("unsafe_statement_count", len(safetyResult.Statements)))...)
				app.apiError(w, r, http.StatusUnprocessableEntity,
					"unsafe_query_confirmation_required",
					"This statement has no WHERE clause and will affect every row. Confirm to run it anyway.",
					response.APIError{Details: safetyResult.Statements},
					nil,
				)
				return
			}
		}
		if input.Explain != "" {
			rs, execErr = executeExplainPlan()
		} else {
			rs, execErr = execStatement()
		}
	case classifier.KindDDL:
		if !hasBroadExecute && !app.policyEvaluator.Can(r.Context(),
			account.ID, org.ID,
			ws.OwnerType, "connection", conn.ID,
			access.PermConnDDL,
		) {
			app.logWarn(r, "query permission denied", append(logAttrs, slog.String("required_permission", access.PermConnDDL))...)
			app.notPermitted(w, r)
			return
		}
		if input.Explain != "" {
			rs, execErr = executeExplainPlan()
		} else {
			rs, execErr = execStatement()
		}
	default:
		if !hasBroadExecute {
			app.logWarn(r, "query permission denied", append(logAttrs, slog.String("required_permission", access.PermConnExecute))...)
			app.notPermitted(w, r)
			return
		}
		if input.Explain != "" {
			rs, execErr = executeExplainPlan()
		} else {
			rs, execErr = execStatement()
		}
	}

	for _, stmt := range explainPlan.Teardown {
		if _, tdErr := app.executionRuntime.Execute(context.WithoutCancel(r.Context()), execution.ExecuteRequest{Handle: handle, SQL: stmt}); tdErr != nil {
			app.logWarn(r, "explain teardown failed", append(logAttrs, slog.String("failure_category", executionErrorCategory(tdErr)))...)
		}
	}

	if execErr != nil {
		if errors.Is(execErr, context.Canceled) || errors.Is(execErr, context.DeadlineExceeded) || r.Context().Err() != nil {
			_ = app.executionRuntime.Cancel(context.WithoutCancel(r.Context()), execution.SessionRequest{Handle: handle})
			app.logDebug(r, "query cancelled", append(logAttrs, slog.Int64("duration_ms", time.Since(start).Milliseconds()))...)
			app.errorMessage(w, r, statusClientClosedRequest, "Query was cancelled.", nil)
			return
		}
		app.logWarn(r, "query execution failed", append(logAttrs,
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			slog.String("failure_category", executionErrorCategory(execErr)),
		)...)
		app.errorMessage(w, r, http.StatusUnprocessableEntity, execErr.Error(), nil)
		return
	}
	if classification.Kind != classifier.KindDML {
		rs.RowsAffected = nil
	}

	rs.DurationMs = time.Since(start).Milliseconds()
	app.logDebug(r, "query executed", append(logAttrs,
		slog.Int64("duration_ms", rs.DurationMs),
		slog.Group("result", "rows", len(rs.Rows), "columns", len(rs.Columns)),
	)...)
	if classification.Kind == classifier.KindDDL {
		if _, _, syncErr := app.enqueueSchemaSync(context.WithoutCancel(r.Context()), conn.ID, ws.OrgID); syncErr != nil &&
			!errors.Is(syncErr, jobs.ErrActiveExists) {
			app.logWarn(r, "post-ddl schema snapshot enqueue failed", append(logAttrs, slog.String("failure_category", "enqueue_failed"))...)
		}
	}

	transactionStatus, statusErr := app.executionRuntime.TransactionStatus(r.Context(), execution.SessionRequest{Handle: handle})
	if statusErr != nil {
		app.serverError(w, r, statusErr)
		return
	}
	err = response.JSON(w, http.StatusOK, struct {
		*result.ResultSet
		Transaction transactionStatusView `json:"transaction"`
	}{ResultSet: rs, Transaction: newTransactionStatusView(transactionStatus)})
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) executeDQLQuery(r *http.Request, handle execution.SessionHandle, sql string, useCursor *bool, pageSize *int, start time.Time, runtimeSettings settingsapp.Effective) (*result.ResultSet, error) {
	if useCursor == nil || *useCursor {
		rs, err := app.executeQueryWithCursor(r, handle, sql, queryCursorPageSize(pageSize, runtimeSettings), start, runtimeSettings)
		if err == nil && rs != nil {
			return rs, nil
		}
		if err != nil && !errors.Is(err, execution.ErrQueryCursorUnsupported) {
			return nil, err
		}
		if errors.Is(err, execution.ErrQueryCursorUnsupported) {
			app.logDebug(r, "query cursor unsupported; falling back to buffered query")
		}
	}
	queried, err := app.executionRuntime.Query(r.Context(), execution.QueryRequest{
		Handle: handle, SQL: sql,
		Limits: execution.Limits{MaxRows: runtimeSettings.QueryMaxResultRows, MaxBytes: runtimeSettings.QueryMaxResultBytes},
	})
	return queried.Result, err
}

func (app *application) executeQueryWithCursor(r *http.Request, handle execution.SessionHandle, sql string, pageSize int, start time.Time, runtimeSettings settingsapp.Effective) (*result.ResultSet, error) {
	app.logDebug(r, "query cursor opening",
		slog.Int("page_size", pageSize),
	)
	queried, err := app.executionRuntime.Query(r.Context(), execution.QueryRequest{
		Handle: handle, SQL: sql, UseCursor: true, PageSize: pageSize,
		Limits: execution.Limits{MaxRows: pageSize, MaxBytes: runtimeSettings.QueryMaxResultBytes},
	})
	if err != nil {
		return nil, err
	}
	rs := queried.Result
	rs.DurationMs = time.Since(start).Milliseconds()
	rs.PageSize = pageSize
	if queried.Exhausted {
		exhausted := true
		rs.Exhausted = &exhausted
	} else {
		exhausted := false
		rs.QueryCursorID = string(queried.Cursor)
		rs.Exhausted = &exhausted
	}
	app.logDebug(r, "query cursor initial page returned",
		queryCursorAttrs(handle, queried.Cursor,
			slog.Int("page_size", pageSize),
			slog.Int("rows_returned", rs.RowsReturned),
			slog.Int64("bytes_returned", rs.BytesReturned),
			slog.Bool("exhausted", queried.Exhausted),
			slog.Bool("truncated", rs.Truncated),
			slog.Int64("duration_ms", rs.DurationMs),
		)...,
	)
	return rs, nil
}
