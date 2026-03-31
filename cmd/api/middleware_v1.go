package main

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/token"
)

// authenticateV1 reads the Bearer token, verifies it, and sets the account on context.
// Continues without error if no token is present — use requireAccount to enforce auth.
func (app *application) authenticateV1(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			next.ServeHTTP(w, r)
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			app.invalidAuthenticationToken(w, r)
			return
		}

		claims, err := token.Verify(parts[1], app.config.jwt.secretKey)
		if err != nil {
			app.invalidAuthenticationToken(w, r)
			return
		}

		accountID, err := strconv.ParseInt(claims.AccountID, 10, 64)
		if err != nil {
			app.invalidAuthenticationToken(w, r)
			return
		}

		account, found, err := app.db.GetAccount(accountID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if !found || !account.IsActive {
			app.invalidAuthenticationToken(w, r)
			return
		}

		r = contextSetAccount(r, account)
		next.ServeHTTP(w, r)
	})
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
		org, found, err := app.db.GetOrgBySlug(slug)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if !found {
			app.notFound(w, r)
			return
		}

		account := contextGetAccount(r)
		isMember, err := app.db.IsOrgMember(org.ID, account.ID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if !isMember {
			app.notPermitted(w, r)
			return
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

		ws, found, err := app.db.GetWorkspace(wsID)
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

		env, found, err := app.db.GetEnvironment(envID)
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

		conn, found, err := app.db.GetConnection(connID)
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

		r = contextSetConnection(r, conn)
		next.ServeHTTP(w, r)
	})
}

// requirePermission checks that the authenticated account holds the given permission
// on the workspace resource in context.
func (app *application) requirePermission(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			account := contextGetAccount(r)
			org := contextGetOrg(r)
			ws := contextGetWorkspace(r)

			ownerType := ws.OwnerType
			resourceType := "workspace"
			resourceID := ws.ID

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

// requireOrgRole checks that the account holds a role with PermOrgWrite (admin) or
// PermOrgTransferOwnership (owner). Use "admin" or "owner" as the roleName parameter.
func (app *application) requireOrgRole(roleName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			account := contextGetAccount(r)
			org := contextGetOrg(r)

			var permission string
			switch roleName {
			case "owner":
				permission = "org:transfer_ownership"
			case "admin":
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
