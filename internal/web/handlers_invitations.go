package web

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/identity"
	"github.com/sqlwarden/internal/orgs"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

type invitationResponse struct {
	Invitation     orgs.Invitation `json:"invitation"`
	InviteURL      string          `json:"invite_url,omitempty"`
	DeliveryStatus string          `json:"delivery_status"`
}

func newInvitationResponse(issued orgs.IssuedInvitation) invitationResponse {
	return invitationResponse{Invitation: issued.Invitation, InviteURL: issued.InviteURL, DeliveryStatus: issued.DeliveryStatus}
}

// invitationError translates invitation failures to the standard error
// envelope.
func (app *application) invitationError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, orgs.ErrInvitationExpired):
		app.errorMessage(w, r, http.StatusGone, "This invitation has expired. Ask an organization administrator to resend it.", nil)
	case errors.Is(err, orgs.ErrAuthenticationRequired):
		app.authenticationRequired(w, r)
	case errors.Is(err, orgs.ErrSignOutRequired):
		app.errorMessage(w, r, http.StatusConflict, "Sign out before creating the invited account.", nil)
	case errors.Is(err, orgs.ErrInvitationUnavailable):
		app.errorMessage(w, r, http.StatusConflict, "This invitation is no longer available.", nil)
	case errors.Is(err, orgs.ErrInvitationAccountExists):
		app.errorMessage(w, r, http.StatusConflict, "An account for this email now exists. Sign in and accept the invitation again.", nil)
	default:
		app.orgError(w, r, err, "")
	}
}

func (app *application) listOrganizationInvitations(w http.ResponseWriter, r *http.Request) {
	if !app.organizationInvitationsAvailable(w, r) {
		return
	}
	q, fieldErrors := readListQuery(r.URL.Query(), map[string]string{"created_at": "created_at"})
	if len(fieldErrors) > 0 {
		v := validator.Validator{FieldErrors: fieldErrors}
		app.failedValidation(w, r, v)
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	page, err := app.services.Orgs.ListInvitations(r.Context(), principal, contextGetPathIDs(r).OrgID, orgs.ListInvitationsParams{
		Search: q.Search, Page: q.Page, PageSize: q.PageSize,
	})
	if err != nil {
		app.invitationError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, page); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) createOrganizationInvitation(w http.ResponseWriter, r *http.Request) {
	if !app.organizationInvitationsAvailable(w, r) {
		return
	}
	var input struct {
		Email string `json:"email"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	ids := contextGetPathIDs(r)
	issued, err := app.services.Orgs.CreateInvitation(r.Context(), principal, contextGetPathIDs(r).OrgID, input.Email)
	if err != nil {
		app.invitationError(w, r, err)
		return
	}
	app.logInfo(r, "organization invitation created", slog.Int64("org_id", ids.OrgID), slog.String("invitation_id", issued.Invitation.ID), slog.String("delivery_status", issued.DeliveryStatus))
	if err := response.JSON(w, http.StatusCreated, newInvitationResponse(issued)); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) resendOrganizationInvitation(w http.ResponseWriter, r *http.Request) {
	if !app.organizationInvitationsAvailable(w, r) {
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	ids := contextGetPathIDs(r)
	issued, err := app.services.Orgs.ResendInvitation(r.Context(), principal, contextGetPathIDs(r).OrgID, chi.URLParam(r, "invitation_id"))
	if err != nil {
		app.invitationError(w, r, err)
		return
	}
	app.logInfo(r, "organization invitation resent", slog.Int64("org_id", ids.OrgID), slog.String("invitation_id", issued.Invitation.ID), slog.String("delivery_status", issued.DeliveryStatus))
	if err := response.JSON(w, http.StatusOK, newInvitationResponse(issued)); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) revokeOrganizationInvitation(w http.ResponseWriter, r *http.Request) {
	if !app.organizationInvitationsAvailable(w, r) {
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	ids := contextGetPathIDs(r)
	invitationID := chi.URLParam(r, "invitation_id")
	if err := app.services.Orgs.RevokeInvitation(r.Context(), principal, contextGetPathIDs(r).OrgID, invitationID); err != nil {
		app.invitationError(w, r, err)
		return
	}
	app.logInfo(r, "organization invitation revoked", slog.Int64("org_id", ids.OrgID), slog.String("invitation_id", invitationID))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) getOrganizationInvitation(w http.ResponseWriter, r *http.Request) {
	if !app.organizationInvitationsAvailable(w, r) {
		return
	}
	view, err := app.services.Orgs.GetInvitation(r.Context(), chi.URLParam(r, "token"), contextGetAccount(r).ID)
	if err != nil {
		app.invitationError(w, r, err)
		return
	}
	payload := map[string]any{
		"organization": view.Organization, "email": view.Email, "expires_at": view.ExpiresAt,
		"status": view.Status, "account_exists": view.AccountExists, "authenticated_as_invitee": view.AuthenticatedAsInvitee,
	}
	if err := response.JSON(w, http.StatusOK, payload); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) acceptOrganizationInvitation(w http.ResponseWriter, r *http.Request) {
	if !app.organizationInvitationsAvailable(w, r) {
		return
	}
	var decodeErr error
	accepted, err := app.services.Orgs.AcceptInvitation(r.Context(), chi.URLParam(r, "token"), contextGetAccount(r).ID, func() (orgs.AcceptInvitationInput, error) {
		var input struct {
			Name     string `json:"name"`
			Password string `json:"password"`
		}
		if err := request.DecodeJSON(w, r, &input); err != nil {
			decodeErr = err
			return orgs.AcceptInvitationInput{}, err
		}
		return orgs.AcceptInvitationInput{Name: input.Name, Password: input.Password}, nil
	})
	if decodeErr != nil {
		app.badRequest(w, r, decodeErr)
		return
	}
	if err != nil {
		app.invitationError(w, r, err)
		return
	}

	result := map[string]any{"organization": accepted.Organization}
	status := http.StatusOK
	if accepted.Created {
		issued, err := app.services.Identity.StartSession(r.Context(), accepted.Account, identity.SessionAuth{Method: identity.MethodPassword, Assurance: string(access.AAL1)}, app.requestAttributes(r))
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		app.setRefreshCookie(w, r, issued)
		result["access_token"] = issued.AccessToken
		status = http.StatusCreated
		app.logInfo(r, "invited account created", slog.Int64("account_id", accepted.Account.ID), slog.Int64("org_id", accepted.Organization.ID), slog.String("auth_session_id", issued.AuthSessionID))
	}
	app.logInfo(r, "organization invitation accepted", slog.Int64("account_id", accepted.Account.ID), slog.Int64("org_id", accepted.Organization.ID))
	if err := response.JSON(w, status, result); err != nil {
		app.serverError(w, r, err)
	}
}

// organizationInvitationsAvailable answers 404 before any other check when
// the profile has no invitations, so their routes look absent.
func (app *application) organizationInvitationsAvailable(w http.ResponseWriter, r *http.Request) bool {
	if !app.invitationPolicy.Enabled() {
		app.notFound(w, r)
		return false
	}
	return true
}
