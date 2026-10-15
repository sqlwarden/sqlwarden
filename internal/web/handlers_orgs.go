package web

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/orgs"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

// orgError translates orgs service failures to the standard error envelope.
// lastOwnerMessage is the validation message for ErrLastOwner.
func (app *application) orgError(w http.ResponseWriter, r *http.Request, err error, lastOwnerMessage string) {
	var validation *orgs.ValidationError
	switch {
	case errors.As(err, &validation):
		app.failedValidation(w, r, fieldErrors(validation.Fields))
	case errors.Is(err, orgs.ErrLastOwner) && lastOwnerMessage != "":
		v := validator.Validator{}
		v.AddError(lastOwnerMessage)
		app.failedValidation(w, r, v)
	default:
		app.serviceError(w, r, err)
	}
}

func (app *application) getOrg(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	org, err := app.services.Orgs.Get(r.Context(), principal, contextGetPathIDs(r).OrgID)
	if err != nil {
		app.orgError(w, r, err, "")
		return
	}
	if err := response.JSON(w, http.StatusOK, org); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) updateOrg(w http.ResponseWriter, r *http.Request) {
	ids := contextGetPathIDs(r)

	var input struct {
		Name                        *string `json:"name"`
		SchemaSnapshotsEnabled      *bool   `json:"schema_snapshots_enabled"`
		AllowConnectionSecretReveal *bool   `json:"allow_connection_secret_reveal"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}

	updated, err := app.services.Orgs.Update(r.Context(), principal, contextGetPathIDs(r).OrgID, orgs.UpdateOrgInput{
		Name:                        input.Name,
		SchemaSnapshotsEnabled:      input.SchemaSnapshotsEnabled,
		AllowConnectionSecretReveal: input.AllowConnectionSecretReveal,
	})
	if err != nil {
		app.orgError(w, r, err, "")
		return
	}
	app.logInfo(r, "organization updated", slog.Int64("org_id", ids.OrgID), slog.String("org_slug", ids.OrgSlug))
	if err := response.JSON(w, http.StatusOK, updated); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) deleteOrg(w http.ResponseWriter, r *http.Request) {
	ids := contextGetPathIDs(r)
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	if err := app.services.Orgs.Delete(r.Context(), principal, contextGetPathIDs(r).OrgID); err != nil {
		app.orgError(w, r, err, "")
		return
	}
	app.logInfo(r, "organization deleted", slog.Int64("org_id", ids.OrgID), slog.String("org_slug", ids.OrgSlug))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) createOrg(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}

	org, err := app.services.Orgs.Create(r.Context(), principal, orgs.CreateOrgInput{Name: input.Name, Slug: input.Slug})
	if err != nil {
		app.orgError(w, r, err, "")
		return
	}
	app.logInfo(r, "organization created", slog.Int64("org_id", org.ID), slog.String("org_slug", org.Slug), slog.Int64("owner_account_id", principal.Subject.ID))
	if err := response.JSON(w, http.StatusCreated, org); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) listOrgMembers(w http.ResponseWriter, r *http.Request) {
	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"name":       "name",
		"email":      "email",
		"created_at": "joined_at",
	})
	role := strings.TrimSpace(r.URL.Query().Get("role"))
	if role != "" && !orgs.ValidMemberRole(role) {
		errs["role"] = "Role must be Owner, Administrator, or Baseline Access."
	}
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}

	members, err := app.services.Orgs.ListMembers(r.Context(), principal, contextGetPathIDs(r).OrgID, orgs.ListMembersParams{
		Search:   q.Search,
		Role:     role,
		Sort:     q.Sort,
		Order:    q.Order,
		Page:     q.Page,
		PageSize: q.PageSize,
	})
	if err != nil {
		app.orgError(w, r, err, "")
		return
	}
	if err := response.JSON(w, http.StatusOK, members); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getOrgMember(w http.ResponseWriter, r *http.Request) {
	accountID, err := strconv.ParseInt(chi.URLParam(r, "account_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	member, err := app.services.Orgs.GetMember(r.Context(), principal, contextGetPathIDs(r).OrgID, accountID)
	if err != nil {
		app.orgError(w, r, err, "")
		return
	}
	if err := response.JSON(w, http.StatusOK, member); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) listOrgMemberTeams(w http.ResponseWriter, r *http.Request) {
	accountID, err := strconv.ParseInt(chi.URLParam(r, "account_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}
	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"name":       "name",
		"created_at": "created_at",
	})
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}

	teams, err := app.services.Orgs.ListMemberTeams(r.Context(), principal, contextGetPathIDs(r).OrgID, accountID, orgs.ListTeamsParams{
		Search:   q.Search,
		Slug:     strings.TrimSpace(r.URL.Query().Get("slug")),
		Sort:     q.Sort,
		Order:    q.Order,
		Page:     q.Page,
		PageSize: q.PageSize,
	})
	if err != nil {
		app.orgError(w, r, err, "")
		return
	}
	if err := response.JSON(w, http.StatusOK, teams); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) removeOrgMember(w http.ResponseWriter, r *http.Request) {
	ids := contextGetPathIDs(r)
	accountID, err := strconv.ParseInt(chi.URLParam(r, "account_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}

	err = app.services.Orgs.RemoveMember(r.Context(), principal, contextGetPathIDs(r).OrgID, accountID)
	if errors.Is(err, orgs.ErrLastOwner) {
		app.logWarn(r, "last organization owner removal blocked", slog.Int64("target_account_id", accountID), slog.Int64("org_id", ids.OrgID), slog.String("org_slug", ids.OrgSlug))
	}
	if err != nil {
		app.orgError(w, r, err, "Cannot remove the last owner of an organization.")
		return
	}
	app.logInfo(r, "organization member removed", slog.Int64("target_account_id", accountID), slog.Int64("org_id", ids.OrgID), slog.String("org_slug", ids.OrgSlug))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) updateOrgMemberRole(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Role string `json:"role"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	ids := contextGetPathIDs(r)
	accountID, err := strconv.ParseInt(chi.URLParam(r, "account_id"), 10, 64)
	if err != nil {
		if !orgs.ValidMemberRole(input.Role) {
			app.orgError(w, r, memberRoleValidation(input.Role), "")
			return
		}
		app.notFound(w, r)
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}

	err = app.services.Orgs.UpdateMemberRole(r.Context(), principal, contextGetPathIDs(r).OrgID, accountID, input.Role)
	if errors.Is(err, orgs.ErrLastOwner) {
		app.logWarn(r, "last organization owner demotion blocked", slog.Int64("target_account_id", accountID), slog.Int64("org_id", ids.OrgID), slog.String("org_slug", ids.OrgSlug), slog.String("requested_role", input.Role))
	}
	if err != nil {
		app.orgError(w, r, err, "Cannot demote the last owner of an organization.")
		return
	}
	app.logInfo(r, "organization member builtin role updated", slog.Int64("target_account_id", accountID), slog.String("role", input.Role))
	w.WriteHeader(http.StatusNoContent)
}

// memberRoleValidation reports an invalid role with the messages the service
// uses, for a request rejected before it reaches the service.
func memberRoleValidation(role string) error {
	message := "Role must be Owner, Administrator, or Baseline Access."
	if role == "" {
		message = "Role is required."
	}
	return &orgs.ValidationError{Fields: map[string]string{"role": message}}
}
