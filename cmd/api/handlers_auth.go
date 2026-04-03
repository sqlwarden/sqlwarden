package main

import (
	"net/http"
	"strconv"
	"time"

	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/password"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/token"
	"github.com/sqlwarden/internal/validator"
)

func (app *application) registerAccount(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string              `json:"email"`
		Name     string              `json:"name"`
		Password string              `json:"password"`
		V        validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.V.CheckField(input.Email != "", "email", "email is required")
	input.V.CheckField(validator.IsEmail(input.Email), "email", "must be a valid email address")
	input.V.CheckField(input.Name != "", "name", "name is required")
	input.V.CheckField(len(input.Password) >= 8, "password", "must be at least 8 characters")

	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	configured, err := app.db.HasAnyInstanceAdmin(r.Context())
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !configured {
		app.errorMessage(w, r, http.StatusForbidden, "instance setup is not complete", nil)
		return
	}

	_, exists, err := app.db.GetAccountByEmail(r.Context(), input.Email)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if exists {
		input.V.AddFieldError("email", "email address is already in use")
		app.failedValidation(w, r, input.V)
		return
	}

	hashedPW, err := password.Hash(input.Password)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	account, err := app.db.InsertAccount(r.Context(), input.Email, input.Name, &hashedPW)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = response.JSON(w, http.StatusCreated, account)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) loginAccount(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string              `json:"email"`
		Password string              `json:"password"`
		V        validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.V.CheckField(input.Email != "", "email", "email is required")
	input.V.CheckField(input.Password != "", "password", "password is required")

	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	account, found, err := app.db.GetAccountByEmail(r.Context(), input.Email)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	if !found || account.Password == nil || !account.IsActive {
		app.invalidAuthenticationToken(w, r)
		return
	}

	match, err := password.Matches(input.Password, *account.Password)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !match {
		app.invalidAuthenticationToken(w, r)
		return
	}

	accountIDStr := strconv.FormatInt(account.ID, 10)
	accessToken, _, err := token.Issue(accountIDStr, account.Email, account.Name, app.config.jwt.secretKey)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	family := database.NewID()
	tokenHash := token.Hash(family)
	_, err = app.db.InsertRefreshToken(r.Context(),
		account.ID,
		tokenHash,
		family,
		time.Now().Add(7*24*time.Hour),
		r.Header.Get("User-Agent"),
		r.RemoteAddr,
	)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    family,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/api/v1/auth",
		MaxAge:   7 * 24 * 3600,
	})

	err = response.JSON(w, http.StatusOK, map[string]string{"access_token": accessToken})
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) refreshToken(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("refresh_token")
	if err != nil {
		app.invalidAuthenticationToken(w, r)
		return
	}

	rt, found, err := app.db.GetRefreshTokenByHash(r.Context(), token.Hash(cookie.Value))
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found || rt.RevokedAt != nil || time.Now().After(rt.ExpiresAt) {
		if found && rt.RevokedAt == nil {
			_ = app.db.RevokeFamilyTokens(r.Context(), rt.Family)
		}
		app.invalidAuthenticationToken(w, r)
		return
	}

	_ = app.db.RevokeRefreshToken(r.Context(), rt.ID)

	account, found, err := app.db.GetAccount(r.Context(), rt.AccountID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.invalidAuthenticationToken(w, r)
		return
	}

	accountIDStr := strconv.FormatInt(account.ID, 10)
	accessToken, _, err := token.Issue(accountIDStr, account.Email, account.Name, app.config.jwt.secretKey)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	family := database.NewID()
	_, err = app.db.InsertRefreshToken(r.Context(),
		rt.AccountID,
		token.Hash(family),
		rt.Family,
		time.Now().Add(7*24*time.Hour),
		r.Header.Get("User-Agent"),
		r.RemoteAddr,
	)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    family,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/api/v1/auth",
		MaxAge:   7 * 24 * 3600,
	})

	err = response.JSON(w, http.StatusOK, map[string]string{"access_token": accessToken})
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) logoutAccount(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("refresh_token")
	if err == nil {
		rt, found, err := app.db.GetRefreshTokenByHash(r.Context(), token.Hash(cookie.Value))
		if err == nil && found {
			_ = app.db.RevokeRefreshToken(r.Context(), rt.ID)
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
	account := contextGetAccount(r)
	err := response.JSON(w, http.StatusOK, account)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getAccountOrgs(w http.ResponseWriter, r *http.Request) {
	account := contextGetAccount(r)
	orgs, err := app.db.GetAccountOrgs(r.Context(), account.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if orgs == nil {
		orgs = []database.Organization{}
	}
	err = response.JSON(w, http.StatusOK, orgs)
	if err != nil {
		app.serverError(w, r, err)
	}
}

// getSession returns the authenticated account plus UI bootstrap metadata.
func (app *application) getSession(w http.ResponseWriter, r *http.Request) {
	account := contextGetAccount(r)

	orgs, err := app.db.GetAccountOrgs(r.Context(), account.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	isAdmin, err := app.db.IsInstanceAdmin(r.Context(), account.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = response.JSON(w, http.StatusOK, map[string]any{
		"account":           account,
		"organizations":     orgs,
		"is_instance_admin": isAdmin,
	})
	if err != nil {
		app.serverError(w, r, err)
	}
}
