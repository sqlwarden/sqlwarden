package web

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/identity"
)

// authenticateV1 authenticates the Authorization header through the identity
// chain and sets the account, auth session and principal on context.
// Continues without error if no credential is present; use requireAccount to enforce auth.
func (app *application) authenticateV1(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			next.ServeHTTP(w, r)
			return
		}
		runtimeSettings, err := app.runtimeSettingsService().effectiveForOrg(r.Context(), nil)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		r = contextSetRuntimeSettings(r, runtimeSettings)

		result, err := app.authChain.Authenticate(r.Context(), identity.Presented{
			Authorization: authHeader,
			Request:       app.requestAttributes(r),
		})
		var credErr *identity.CredentialError
		if errors.As(err, &credErr) {
			attrs := []slog.Attr{slog.String("reason", credErr.Reason)}
			if credErr.AccountID != 0 {
				attrs = append(attrs, slog.Int64("account_id", credErr.AccountID))
			}
			if credErr.CredentialID != "" {
				attrs = append(attrs, slog.String("auth_session_id", credErr.CredentialID))
			}
			app.logWarn(r, "authentication rejected", attrs...)
			app.rejectCredential(w, r, credErr.Reason)
			return
		}
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if result.AuthSession != nil {
			r = contextSetAuthSession(r, *result.AuthSession)
		}
		r = contextSetAccount(r, result.Account)
		r = contextSetPrincipal(r, result.Principal)
		next.ServeHTTP(w, r)
	})
}

func revocationFromContext(ctx context.Context) bool {
	settings, ok := ctx.Value(runtimeSettingsKey).(effectiveRuntimeSettings)
	return ok && settings.SessionsRevocationEnabled
}

// requireAccount rejects the request with 401 if no authenticated account is in context.
func (app *application) requireAccount(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account := contextGetAccount(r)
		if account.ID == 0 {
			app.authenticationRequired(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// orgCtx resolves the org from the URL slug and sets it on context.
// Requires requireAccount to have run first.
func (app *application) orgCtx(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "org_slug")
		org, found, err := app.db.GetOrgBySlug(r.Context(), slug)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if !found {
			app.notFound(w, r)
			return
		}

		account := contextGetAccount(r)
		isMember, err := app.db.IsOrgMember(r.Context(), org.ID, account.ID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if !isMember {
			app.notPermitted(w, r)
			return
		}

		runtimeSettings, ok := contextGetRuntimeSettings(r)
		if !ok {
			runtimeSettings, err = app.runtimeSettingsService().effectiveForOrg(r.Context(), nil)
			if err != nil {
				app.serverError(w, r, err)
				return
			}
		}
		if runtimeSettings.SessionsRevocationEnabled {
			authSession := contextGetAuthSession(r)
			if authSession.ID == "" {
				app.invalidAuthenticationToken(w, r)
				return
			}
			err = app.db.EnsureOrgAccessSession(r.Context(), authSession.ID, org.ID, account.ID, authSession.ExpiresAt)
			if err != nil {
				app.serverError(w, r, err)
				return
			}
			orgAccessSession, found, err := app.db.GetOrgAccessSession(r.Context(), authSession.ID, org.ID, account.ID)
			if err != nil {
				app.serverError(w, r, err)
				return
			}
			if !found || orgAccessSession.RevokedAt != nil || time.Now().After(orgAccessSession.ExpiresAt) {
				reason := "org_access_session_not_found"
				if found && orgAccessSession.RevokedAt != nil {
					reason = "org_access_session_revoked"
				} else if found && time.Now().After(orgAccessSession.ExpiresAt) {
					reason = "org_access_session_expired"
				}
				app.logWarn(r, "organization access session rejected", slog.Int64("account_id", account.ID), slog.Int64("org_id", org.ID), slog.String("auth_session_id", authSession.ID), slog.String("reason", reason))
				app.notPermitted(w, r)
				return
			}
		}

		r = contextSetOrg(r, org)
		next.ServeHTTP(w, r)
	})
}

// wsCtx resolves the workspace from the URL parameter and sets it on context.
func (app *application) wsCtx(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wsIDStr := chi.URLParam(r, "ws_id")
		wsID, err := strconv.ParseInt(wsIDStr, 10, 64)
		if err != nil {
			app.notFound(w, r)
			return
		}

		ws, found, err := app.db.GetWorkspace(r.Context(), wsID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if !found {
			app.notFound(w, r)
			return
		}

		org := contextGetOrg(r)
		if ws.OrgID == nil || *ws.OrgID != org.ID {
			app.notFound(w, r)
			return
		}

		r = contextSetWorkspace(r, ws)
		next.ServeHTTP(w, r)
	})
}

// envCtx resolves the environment from the URL parameter and sets it on context.
func (app *application) envCtx(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		envIDStr := chi.URLParam(r, "env_id")
		envID, err := strconv.ParseInt(envIDStr, 10, 64)
		if err != nil {
			app.notFound(w, r)
			return
		}

		env, found, err := app.db.GetEnvironment(r.Context(), envID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if !found {
			app.notFound(w, r)
			return
		}

		ws := contextGetWorkspace(r)
		if env.WorkspaceID != ws.ID {
			app.notFound(w, r)
			return
		}

		r = contextSetEnvironment(r, env)
		next.ServeHTTP(w, r)
	})
}

// connCtx resolves the connection from the URL parameter and sets it on context.
func (app *application) connCtx(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connIDStr := chi.URLParam(r, "conn_id")
		connID, err := strconv.ParseInt(connIDStr, 10, 64)
		if err != nil {
			app.notFound(w, r)
			return
		}

		conn, found, err := app.db.GetConnection(r.Context(), connID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if !found {
			app.notFound(w, r)
			return
		}

		ws := contextGetWorkspace(r)
		if conn.WorkspaceID != ws.ID {
			app.notFound(w, r)
			return
		}
		env := contextGetEnvironment(r)
		if env.ID != 0 && conn.EnvironmentID != env.ID {
			app.notFound(w, r)
			return
		}

		r = contextSetConnection(r, conn)
		next.ServeHTTP(w, r)
	})
}

func (app *application) requireOrgPermission(permission string) func(http.Handler) http.Handler {
	return app.requireResourcePermission(permission, func(r *http.Request) (string, string, int64, bool) {
		org := contextGetOrg(r)
		return "org", "org", org.ID, org.ID != 0
	})
}

func (app *application) requireWorkspacePermission(permission string) func(http.Handler) http.Handler {
	return app.requireResourcePermission(permission, func(r *http.Request) (string, string, int64, bool) {
		ws := contextGetWorkspace(r)
		return ws.OwnerType, "workspace", ws.ID, ws.ID != 0
	})
}

func (app *application) requireEnvironmentPermission(permission string) func(http.Handler) http.Handler {
	return app.requireResourcePermission(permission, func(r *http.Request) (string, string, int64, bool) {
		ws := contextGetWorkspace(r)
		env := contextGetEnvironment(r)
		return ws.OwnerType, "environment", env.ID, ws.ID != 0 && env.ID != 0
	})
}

func (app *application) requireConnectionPermission(permission string) func(http.Handler) http.Handler {
	return app.requireResourcePermission(permission, func(r *http.Request) (string, string, int64, bool) {
		ws := contextGetWorkspace(r)
		conn := contextGetConnection(r)
		return ws.OwnerType, "connection", conn.ID, ws.ID != 0 && conn.ID != 0
	})
}

func (app *application) requireResourcePermission(permission string, resource func(*http.Request) (string, string, int64, bool)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ownerType, resourceType, resourceID, ok := resource(r)
			if !ok {
				app.notFound(w, r)
				return
			}
			account := contextGetAccount(r)
			org := contextGetOrg(r)
			if org.ID == 0 {
				app.notFound(w, r)
				return
			}
			allowed := app.enforcer.Can(r.Context(),
				account.ID, org.ID,
				ownerType, resourceType, resourceID,
				permission,
			)
			if !allowed {
				app.notPermitted(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// requireInstanceAdmin rejects with 403 if the authenticated account is not an instance admin.
func (app *application) requireInstanceAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account := contextGetAccount(r)
		isAdmin, err := app.db.IsInstanceAdmin(r.Context(), account.ID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if !isAdmin {
			app.notPermitted(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireOrgRole checks that the account holds a role with PermOrgWrite (admin) or
// PermOrgTransferOwnership (owner).
func (app *application) requireOrgRole(roleName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			account := contextGetAccount(r)
			org := contextGetOrg(r)

			var permission string
			switch roleName {
			case access.BuiltinOrgOwnerRole:
				permission = "org:transfer_ownership"
			case access.BuiltinOrgAdminRole:
				permission = "org:write"
			default:
				app.notPermitted(w, r)
				return
			}

			allowed := app.enforcer.Can(r.Context(),
				account.ID, org.ID,
				"org", "org", org.ID,
				permission,
			)
			if !allowed {
				app.notPermitted(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
