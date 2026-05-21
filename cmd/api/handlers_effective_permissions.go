package main

import (
	"net/http"
	"strconv"
	"strings"

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

	resourceID, ok := app.resolveEffectivePermissionResource(w, r, resourceType)
	if !ok {
		return
	}

	permissions, err := app.enforcer.EffectivePermissions(r.Context(),
		account.ID, org.ID,
		"org", resourceType, resourceID,
	)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = response.JSON(w, http.StatusOK, effectivePermissionsResponse{
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Permissions:  permissions,
	})
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) resolveEffectivePermissionResource(w http.ResponseWriter, r *http.Request, resourceType string) (int64, bool) {
	org := contextGetOrg(r)

	switch resourceType {
	case "org":
		rawID := strings.TrimSpace(r.URL.Query().Get("resource_id"))
		if rawID == "" {
			return org.ID, true
		}
		resourceID, ok := app.parseEffectivePermissionResourceID(w, r, rawID)
		if !ok {
			return 0, false
		}
		if resourceID != org.ID {
			app.notFound(w, r)
			return 0, false
		}
		return resourceID, true

	case "workspace":
		resourceID, ok := app.requiredEffectivePermissionResourceID(w, r)
		if !ok {
			return 0, false
		}
		ws, found, err := app.db.GetWorkspace(r.Context(), resourceID)
		if err != nil {
			app.serverError(w, r, err)
			return 0, false
		}
		if !found || ws.OrgID == nil || *ws.OrgID != org.ID {
			app.notFound(w, r)
			return 0, false
		}
		return resourceID, true

	case "environment":
		resourceID, ok := app.requiredEffectivePermissionResourceID(w, r)
		if !ok {
			return 0, false
		}
		env, found, err := app.db.GetEnvironment(r.Context(), resourceID)
		if err != nil {
			app.serverError(w, r, err)
			return 0, false
		}
		if !found {
			app.notFound(w, r)
			return 0, false
		}
		ws, found, err := app.db.GetWorkspace(r.Context(), env.WorkspaceID)
		if err != nil {
			app.serverError(w, r, err)
			return 0, false
		}
		if !found || ws.OrgID == nil || *ws.OrgID != org.ID {
			app.notFound(w, r)
			return 0, false
		}
		return resourceID, true

	case "connection":
		resourceID, ok := app.requiredEffectivePermissionResourceID(w, r)
		if !ok {
			return 0, false
		}
		conn, found, err := app.db.GetConnection(r.Context(), resourceID)
		if err != nil {
			app.serverError(w, r, err)
			return 0, false
		}
		if !found {
			app.notFound(w, r)
			return 0, false
		}
		ws, found, err := app.db.GetWorkspace(r.Context(), conn.WorkspaceID)
		if err != nil {
			app.serverError(w, r, err)
			return 0, false
		}
		if !found || ws.OrgID == nil || *ws.OrgID != org.ID {
			app.notFound(w, r)
			return 0, false
		}
		return resourceID, true

	default:
		app.failedValidation(w, r, fieldErrors(map[string]string{
			"resource_type": "Resource type must be org, workspace, environment, or connection.",
		}))
		return 0, false
	}
}

func (app *application) requiredEffectivePermissionResourceID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	rawID := strings.TrimSpace(r.URL.Query().Get("resource_id"))
	if rawID == "" {
		app.failedValidation(w, r, fieldErrors(map[string]string{
			"resource_id": "Resource is required.",
		}))
		return 0, false
	}
	return app.parseEffectivePermissionResourceID(w, r, rawID)
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
