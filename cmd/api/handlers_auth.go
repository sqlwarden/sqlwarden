package main

import (
	"crypto/rand"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/oklog/ulid/v2"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/password"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/token"
	"github.com/sqlwarden/internal/validator"
)

var rgxSlugChar = regexp.MustCompile(`[^a-z0-9-]`)

func (app *application) registerAccount(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	var v validator.Validator

	v.CheckField(validator.NotBlank(input.Email), "email", "Email is required")
	v.CheckField(validator.Matches(input.Email, validator.RgxEmail), "email", "Must be a valid email address")
	v.CheckField(validator.NotBlank(input.Name), "name", "Name is required")
	v.CheckField(validator.MaxRunes(input.Name, 100), "name", "Must not be more than 100 characters")
	v.CheckField(validator.NotBlank(input.Password), "password", "Password is required")
	v.CheckField(validator.MinRunes(input.Password, 8), "password", "Must be at least 8 characters")
	v.CheckField(validator.MaxRunes(input.Password, 72), "password", "Must not be more than 72 characters")
	v.CheckField(validator.NotIn(input.Password, password.CommonPasswords...), "password", "Password is too common")

	if !v.HasErrors() {
		_, exists, err := app.db.GetAccountByEmail(input.Email)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		v.CheckField(!exists, "email", "Email is already in use")
	}

	if v.HasErrors() {
		app.failedValidation(w, r, v)
		return
	}

	hashedPw, err := password.Hash(input.Password)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	account, err := app.db.InsertAccount(input.Email, input.Name, &hashedPw)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Generate personal org slug from email username
	slug := slugFromEmail(input.Email)

	// Check uniqueness
	_, found, err := app.db.GetTenantBySlug(slug)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if found {
		id := ulid.MustNew(ulid.Now(), rand.Reader)
		slug = slug + "-" + strings.ToLower(id.String()[:8])
	}

	tenant, err := app.db.InsertTenant(slug, input.Name+" (Personal)")
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = app.db.AddTenantMember(tenant.ID, account.ID, "owner")
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = app.enforcer.SeedOrgPolicies(tenant.Slug, account.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = response.JSON(w, http.StatusCreated, map[string]any{
		"id":    account.ID,
		"email": account.Email,
		"name":  account.Name,
	})
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) loginAccount(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		OrgSlug  string `json:"org_slug"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	// SSO redirect check
	if input.OrgSlug != "" {
		tenant, found, err := app.db.GetTenantBySlug(input.OrgSlug)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if found {
			_, idpFound, err := app.db.GetTenantIDPConfig(tenant.ID)
			if err != nil {
				app.serverError(w, r, err)
				return
			}
			if idpFound {
				err = response.JSON(w, http.StatusNotImplemented, map[string]string{"error": "SSO not yet implemented"})
				if err != nil {
					app.serverError(w, r, err)
				}
				return
			}
		}
	}

	var v validator.Validator

	v.CheckField(validator.NotBlank(input.Email), "email", "Email is required")
	v.CheckField(validator.NotBlank(input.Password), "password", "Password is required")

	if v.HasErrors() {
		app.failedValidation(w, r, v)
		return
	}

	account, found, err := app.db.GetAccountByEmail(input.Email)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		v.AddFieldError("email", "Invalid credentials")
		app.failedValidation(w, r, v)
		return
	}

	if account.Password == nil {
		v.AddFieldError("email", "Invalid credentials")
		app.failedValidation(w, r, v)
		return
	}

	match, err := password.Matches(input.Password, *account.Password)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !match {
		v.AddFieldError("email", "Invalid credentials")
		app.failedValidation(w, r, v)
		return
	}

	accessToken, expiresAt, err := token.Issue(account.ID, account.Email, account.Name, app.config.jwt.secretKey)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	plainRefresh, hashRefresh, err := token.Generate()
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	familyID := ulid.MustNew(ulid.Now(), rand.Reader).String()

	_, err = app.db.InsertRefreshToken(
		account.ID,
		hashRefresh,
		familyID,
		time.Now().Add(30*24*time.Hour),
		r.UserAgent(),
		r.RemoteAddr,
	)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    plainRefresh,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/api/v1/auth",
		MaxAge:   30 * 24 * 3600,
	})

	err = response.JSON(w, http.StatusOK, map[string]any{
		"access_token": accessToken,
		"expires_at":   expiresAt.UTC().Format(time.RFC3339),
	})
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) refreshToken(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("refresh_token")
	if err != nil {
		app.authenticationRequired(w, r)
		return
	}

	rt, found, err := app.db.GetRefreshTokenByHash(token.Hash(cookie.Value))
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.authenticationRequired(w, r)
		return
	}

	// Reuse detection
	if rt.RevokedAt != nil {
		_ = app.db.RevokeFamilyTokens(rt.Family)
		err = response.JSON(w, http.StatusUnauthorized, map[string]string{
			"error": "Token reuse detected, all sessions invalidated",
		})
		if err != nil {
			app.serverError(w, r, err)
		}
		return
	}

	if rt.ExpiresAt.Before(time.Now()) {
		app.authenticationRequired(w, r)
		return
	}

	// Revoke the used token
	err = app.db.RevokeRefreshToken(rt.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Fetch account for claims
	account, found, err := app.db.GetAccount(rt.AccountID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.authenticationRequired(w, r)
		return
	}

	accessToken, expiresAt, err := token.Issue(account.ID, account.Email, account.Name, app.config.jwt.secretKey)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	plainRefresh, hashRefresh, err := token.Generate()
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	_, err = app.db.InsertRefreshToken(
		rt.AccountID,
		hashRefresh,
		rt.Family,
		time.Now().Add(30*24*time.Hour),
		r.UserAgent(),
		r.RemoteAddr,
	)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    plainRefresh,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/api/v1/auth",
		MaxAge:   30 * 24 * 3600,
	})

	err = response.JSON(w, http.StatusOK, map[string]any{
		"access_token": accessToken,
		"expires_at":   expiresAt.UTC().Format(time.RFC3339),
	})
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) logoutAccount(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("refresh_token")
	if err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	rt, found, err := app.db.GetRefreshTokenByHash(token.Hash(cookie.Value))
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if found && rt.RevokedAt == nil {
		err = app.db.RevokeRefreshToken(rt.ID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    "",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/api/v1/auth",
		MaxAge:   -1,
	})

	w.WriteHeader(http.StatusNoContent)
}

func (app *application) getAccount(w http.ResponseWriter, r *http.Request) {
	account, _ := contextGetAccount(r)

	err := response.JSON(w, http.StatusOK, account)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getAccountOrgs(w http.ResponseWriter, r *http.Request) {
	account, _ := contextGetAccount(r)

	tenants, err := app.db.GetAccountTenants(account.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	if tenants == nil {
		tenants = []database.Tenant{}
	}

	err = response.JSON(w, http.StatusOK, tenants)
	if err != nil {
		app.serverError(w, r, err)
	}
}

// slugFromEmail extracts the part before @ and sanitizes it to [a-z0-9-], max 30 chars.
func slugFromEmail(email string) string {
	parts := strings.SplitN(email, "@", 2)
	slug := strings.ToLower(parts[0])
	slug = rgxSlugChar.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")

	if utf8.RuneCountInString(slug) > 30 {
		slug = slug[:30]
		slug = strings.TrimRight(slug, "-")
	}

	if slug == "" {
		slug = "user"
	}

	return slug
}
