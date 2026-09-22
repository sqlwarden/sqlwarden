package web

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/response"
)

type effectivePermissionsResponse struct {
	ResourceType string   `json:"resource_type"`
	ResourceID   int64    `json:"resource_id"`
	Permissions  []string `json:"permissions"`
}

func (app *application) getEffectivePermissions(w http.ResponseWriter, r *http.Request) {
	account := contextGetAccount(r)
	org := contextGetOrg(r)

	resourceType := strings.TrimSpace(r.URL.Query().Get("resource_type"))
	if resourceType == "" {
		resourceType = "org"
	}

	resourceID := int64(0)
	rawResourceID := strings.TrimSpace(r.URL.Query().Get("resource_id"))
	if resourceType != "org" || rawResourceID != "" {
		var ok bool
		resourceID, ok = app.parseEffectivePermissionResourceID(w, r, rawResourceID)
		if !ok {
			return
		}
	}

	resourceID, permissions, err := app.accessService.EffectivePermissions(r.Context(), access.EffectivePermissionsInput{
		AccountID: account.ID, OrgID: org.ID, ResourceType: resourceType, ResourceID: resourceID,
	})
	if errors.Is(err, access.ErrResourceNotFound) || errors.Is(err, access.ErrSubjectNotFound) {
		app.notFound(w, r)
		return
	}
	if errors.Is(err, access.ErrInvalidResourceType) {
		app.failedValidation(w, r, fieldErrors(map[string]string{
			"resource_type": "Resource type must be org, workspace, environment, or connection.",
		}))
		return
	}
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.logDebug(r, "effective permissions resolved",
		slog.String("resource_type", resourceType),
		slog.Int64("resource_id", resourceID),
		slog.Int("permission_count", len(permissions)),
	)
	err = response.JSON(w, http.StatusOK, effectivePermissionsResponse{
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Permissions:  permissions,
	})
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) parseEffectivePermissionResourceID(w http.ResponseWriter, r *http.Request, rawID string) (int64, bool) {
	if rawID == "" {
		app.failedValidation(w, r, fieldErrors(map[string]string{
			"resource_id": "Resource is required.",
		}))
		return 0, false
	}
	resourceID, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || resourceID <= 0 {
		app.failedValidation(w, r, fieldErrors(map[string]string{
			"resource_id": "Resource must be a positive integer.",
		}))
		return 0, false
	}
	return resourceID, true
}
