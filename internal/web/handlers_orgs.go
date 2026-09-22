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
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
	"github.com/uptrace/bun"
)

const (
	singleUserDefaultOrgName = "Local"
	singleUserDefaultOrgSlug = "local"

	maxOrganizationSlugLength = catalog.MaxOrgSlugLength
)

// createOwnedOrganizationWithExecutor seeds an organization inside a caller's
// transaction. Instance setup composes organization creation with account and
// instance-admin creation in one unit of work, which is why it does not go
// through the catalog's own transactional create.
func (app *application) createOwnedOrganizationWithExecutor(ctx context.Context, tx bun.Tx, slug, name string, ownerAccountID int64) (database.Organization, error) {
	org, err := app.db.InsertOrgWithExecutor(ctx, tx, slug, name)
	if err != nil {
		return database.Organization{}, err
	}
	if err = app.db.AddOrgMemberWithExecutor(ctx, tx, org.ID, ownerAccountID); err != nil {
		return database.Organization{}, err
	}
	if err = app.enforcer.SeedOrgWithExecutor(ctx, tx, org.ID, ownerAccountID); err != nil {
		return database.Organization{}, err
	}
	return org, nil
}

func (app *application) getOrg(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	err := response.JSON(w, http.StatusOK, org)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) updateOrg(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)

	var input struct {
		Name                            *string `json:"name"`
		SchemaSnapshotsEnabled          *bool   `json:"schema_snapshots_enabled"`
		MaskConnectionCredentialsOnEdit *bool   `json:"mask_connection_credentials_on_edit"`
	}

	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}

	result, err := app.catalogService().UpdateOrganization(r.Context(), catalogActor(r), org, catalog.UpdateOrganizationInput{
		Name:                            input.Name,
		SchemaSnapshotsEnabled:          input.SchemaSnapshotsEnabled,
		MaskConnectionCredentialsOnEdit: input.MaskConnectionCredentialsOnEdit,
	})
	if err != nil {
		app.catalogError(w, r, err)
		return
	}
	if result.SnapshotsDisabled {
		if err := app.disableOrganizationSnapshots(r.Context(), org.ID); err != nil {
			app.serverError(w, r, err)
			return
		}
	}
	app.logInfo(r, "organization updated", slog.Int64("org_id", org.ID), slog.String("org_slug", org.Slug))

	if err := response.JSON(w, http.StatusOK, result.Organization); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) deleteOrg(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)

	if err := app.catalogService().DeleteOrganization(r.Context(), catalogActor(r), org); err != nil {
		app.catalogError(w, r, err)
		return
	}

	app.logInfo(r, "organization deleted", slog.Int64("org_id", org.ID), slog.String("org_slug", org.Slug))
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

	account := contextGetAccount(r)
	org, err := app.catalogService().CreateOrganization(r.Context(), catalog.CreateOrganizationInput{
		Name:           input.Name,
		Slug:           input.Slug,
		OwnerAccountID: account.ID,
	})
	switch {
	case errors.Is(err, catalog.ErrSlugTaken):
		app.failedDuplicateField(w, r, "slug", "An organization with this slug already exists.")
		return
	case errors.Is(err, catalog.ErrNameTaken):
		app.failedDuplicateField(w, r, "name", "An organization with this name already exists.")
		return
	case err != nil:
		app.catalogError(w, r, err)
		return
	}

	app.logInfo(r, "organization created", slog.Int64("org_id", org.ID), slog.String("org_slug", org.Slug), slog.Int64("owner_account_id", account.ID))
	if err := response.JSON(w, http.StatusCreated, org); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) listOrgMembers(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"name":       "name",
		"email":      "email",
		"created_at": "joined_at",
	})
	role := strings.TrimSpace(r.URL.Query().Get("role"))
	if role != "" && role != access.BuiltinOrgOwnerRole && role != access.BuiltinOrgAdminRole && role != access.BuiltinOrgMemberRole {
		errs["role"] = "Role must be Owner, Administrator, or Baseline Access."
	}
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}

	members, err := app.catalogService().OrganizationMembers(r.Context(), database.ListOrgMembersParams{
		OrgID:    org.ID,
		Search:   q.Search,
		Role:     role,
		Sort:     q.Sort,
		Order:    q.Order,
		Page:     q.Page,
		PageSize: q.PageSize,
	})
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	err = response.JSON(w, http.StatusOK, members)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getOrgMember(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	accountID, err := strconv.ParseInt(chi.URLParam(r, "account_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}

	member, err := app.catalogService().OrganizationMember(r.Context(), org.ID, accountID)
	if err != nil {
		app.catalogError(w, r, err)
		return
	}
	err = response.JSON(w, http.StatusOK, member)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) listOrgMemberTeams(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	accountID, err := strconv.ParseInt(chi.URLParam(r, "account_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}

	isMember, err := app.db.IsOrgMember(r.Context(), org.ID, accountID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !isMember {
		app.notFound(w, r)
		return
	}

	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"name":       "name",
		"created_at": "created_at",
	})
	slug := strings.TrimSpace(r.URL.Query().Get("slug"))
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}

	teams, err := app.db.ListAccountTeamsPage(r.Context(), database.ListTeamsParams{
		OrgID:    org.ID,
		Search:   q.Search,
		Slug:     slug,
		Sort:     q.Sort,
		Order:    q.Order,
		Page:     q.Page,
		PageSize: q.PageSize,
	}, accountID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	err = response.JSON(w, http.StatusOK, teams)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) removeOrgMember(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	accountID, err := strconv.ParseInt(chi.URLParam(r, "account_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}

	if err := app.catalogService().RemoveOrgMember(r.Context(), catalogActor(r), org.ID, accountID); err != nil {
		if errors.Is(err, catalog.ErrLastOwner) {
			app.logWarn(r, "last organization owner removal blocked", slog.Int64("target_account_id", accountID), slog.Int64("org_id", org.ID), slog.String("org_slug", org.Slug))
		}
		app.catalogError(w, r, err)
		return
	}

	app.logInfo(r, "organization member removed", slog.Int64("target_account_id", accountID), slog.Int64("org_id", org.ID), slog.String("org_slug", org.Slug))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) updateOrgMemberRole(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Role string              `json:"role"`
		V    validator.Validator `json:"-"`
	}

	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.V.CheckField(input.Role != "", "role", "Role is required.")
	input.V.CheckField(catalog.IsBuiltinOrgRole(input.Role), "role", "Role must be Owner, Administrator, or Baseline Access.")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	org := contextGetOrg(r)
	accountID, err := strconv.ParseInt(chi.URLParam(r, "account_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}

	if err := app.catalogService().SetOrgMemberRole(r.Context(), catalogActor(r), org.ID, accountID, input.Role); err != nil {
		if errors.Is(err, catalog.ErrLastOwner) {
			app.logWarn(r, "last organization owner demotion blocked", slog.Int64("target_account_id", accountID), slog.Int64("org_id", org.ID), slog.String("org_slug", org.Slug), slog.String("requested_role", input.Role))
			v := validator.Validator{}
			v.AddError("Cannot demote the last owner of an organization.")
			app.failedValidation(w, r, v)
			return
		}
		app.catalogError(w, r, err)
		return
	}

	app.logInfo(r, "organization member builtin role updated", slog.Int64("target_account_id", accountID), slog.Int64("org_id", org.ID), slog.String("role", input.Role))
	w.WriteHeader(http.StatusNoContent)
}

// slugify converts a name to a URL-safe slug using the catalog's rule, so
// setup, teams, and organization creation cannot drift apart.
func slugify(name string) string { return catalog.Slugify(name) }

// isValidSlug reports whether s is a valid slug under the catalog's rule.
func isValidSlug(s string) bool { return catalog.IsValidSlug(s) }
