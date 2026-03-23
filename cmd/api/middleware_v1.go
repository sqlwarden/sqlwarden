package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/token"
)

// authenticateV1 validates an HS256 JWT from the Authorization header.
// On valid token: fetches Account from DB and sets it in context.
// On missing/invalid: passes through with no account set (unauthenticated is allowed here).
func (app *application) authenticateV1(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Authorization")

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			next.ServeHTTP(w, r)
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			next.ServeHTTP(w, r)
			return
		}

		claims, err := token.Verify(parts[1], app.config.jwt.secretKey)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}

		account, found, err := app.db.GetAccount(claims.AccountID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if found && account.IsActive {
			r = contextSetAccount(r, account)
		}

		next.ServeHTTP(w, r)
	})
}

// requireAccount returns 401 if no authenticated account is in context.
func (app *application) requireAccount(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, ok := contextGetAccount(r)
		if !ok {
			app.authenticationRequired(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// orgCtx resolves {org_slug} to a Tenant and verifies membership.
// Unknown slug -> 404. Not a member -> 403. Sets tenant in context on success.
func (app *application) orgCtx(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "org_slug")

		tenant, found, err := app.db.GetTenantBySlug(slug)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if !found {
			app.notFound(w, r)
			return
		}

		account, ok := contextGetAccount(r)
		if !ok {
			app.authenticationRequired(w, r)
			return
		}

		isMember, err := app.db.IsTenantMember(tenant.ID, account.ID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if !isMember {
			app.notPermitted(w, r)
			return
		}

		r = contextSetTenant(r, tenant)
		next.ServeHTTP(w, r)
	})
}

// wsCtx resolves {ws_id} to a Workspace and verifies it belongs to the current tenant.
// Unknown or cross-tenant workspace -> 404.
func (app *application) wsCtx(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wsID := chi.URLParam(r, "ws_id")

		ws, found, err := app.db.GetWorkspace(wsID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if !found {
			app.notFound(w, r)
			return
		}

		tenant, ok := contextGetTenant(r)
		if !ok || ws.TenantID != tenant.ID {
			app.notFound(w, r)
			return
		}

		r = contextSetWorkspace(r, ws)
		next.ServeHTTP(w, r)
	})
}

// requireOrgRole returns middleware that checks the account holds at least the given built-in role
// in the current org. Uses enforcer.Can with the org-level permission objects.
// role: "owner" | "admin" | "member"
func (app *application) requireOrgRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			account, ok := contextGetAccount(r)
			if !ok {
				app.authenticationRequired(w, r)
				return
			}
			tenant, ok := contextGetTenant(r)
			if !ok {
				app.serverError(w, r, fmt.Errorf("requireOrgRole: orgCtx not in middleware chain"))
				return
			}

			var allowed bool
			switch role {
			case "member":
				// orgCtx already verified membership; all members pass
				allowed = true
			case "admin":
				allowed = app.enforcer.Can(account.ID, tenant.Slug, "members", "write") ||
					app.enforcer.Can(account.ID, tenant.Slug, "*", "*")
			case "owner":
				allowed = app.enforcer.Can(account.ID, tenant.Slug, "*", "*")
			}

			if !allowed {
				app.notPermitted(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// requireSuperadmin returns 403 if the authenticated account is not a superadmin.
func (app *application) requireSuperadmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := contextGetAccount(r)
		if !ok || !account.IsSuperadmin {
			app.errorMessage(w, r, http.StatusForbidden, "superadmin access required", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requirePermission returns middleware that calls enforcer.Can() for a fixed obj+act.
func (app *application) requirePermission(obj, act string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			account, ok := contextGetAccount(r)
			if !ok {
				app.authenticationRequired(w, r)
				return
			}
			tenant, ok := contextGetTenant(r)
			if !ok {
				app.serverError(w, r, fmt.Errorf("requirePermission: orgCtx not in middleware chain"))
				return
			}
			if !app.enforcer.Can(account.ID, tenant.Slug, obj, act) {
				app.notPermitted(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
