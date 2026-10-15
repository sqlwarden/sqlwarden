package web

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/sqlwarden/internal/rbac"
	"github.com/sqlwarden/internal/response"
)

func (app *application) getEffectivePermissions(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	ids := contextGetPathIDs(r)

	result, err := app.services.RBAC.EffectivePermissions(r.Context(), principal, ids.OrgID, rbac.ResourceRef{
		Type: r.URL.Query().Get("resource_type"),
		ID:   r.URL.Query().Get("resource_id"),
	})
	var rejected *rbac.ResourceRejectedError
	if errors.As(err, &rejected) {
		attrs := []slog.Attr{
			slog.String("resource_type", rejected.ResourceType),
			slog.Int64("resource_id", rejected.ResourceID),
		}
		if rejected.Reason != "environment_not_found" && rejected.Reason != "connection_not_found" {
			attrs = append(attrs, slog.Int64("org_id", ids.OrgID))
		}
		app.logWarn(r, "effective permissions resource rejected", append(attrs, slog.String("reason", rejected.Reason))...)
	}
	if err != nil {
		app.rbacError(w, r, err)
		return
	}

	app.logDebug(r, "effective permissions resolved",
		slog.String("resource_type", result.ResourceType),
		slog.Int64("resource_id", result.ResourceID),
		slog.Int("permission_count", len(result.Permissions)),
	)
	if err := response.JSON(w, http.StatusOK, result); err != nil {
		app.serverError(w, r, err)
	}
}
