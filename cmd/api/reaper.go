package main

import (
	"context"
	"strings"
	"time"

	"github.com/sqlwarden/internal/database"
)

func (app *application) startGrantReaper(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			grants, err := app.db.GetExpiredAccessGrants()
			if err != nil {
				app.logger.Error("grant reaper: fetch failed", "error", err)
				continue
			}
			for _, g := range grants {
				if err := app.revokeExpiredGrant(g); err != nil {
					app.logger.Error("grant reaper: revoke failed", "grant_id", g.ID, "error", err)
				}
			}
		}
	}
}

func (app *application) revokeExpiredGrant(g database.AccessGrant) error {
	tenant, found, err := app.db.GetTenant(g.TenantID)
	if err != nil {
		return err
	}
	if !found {
		return nil // tenant deleted, grant orphaned — skip
	}

	slug := tenant.Slug

	switch {
	case strings.HasPrefix(g.Object, "workspace:"):
		wsID := strings.TrimPrefix(g.Object, "workspace:")
		_ = app.enforcer.RevokeWorkspaceAccess(g.Subject, slug, wsID)
	case strings.HasPrefix(g.Object, "connection:"):
		connID := strings.TrimPrefix(g.Object, "connection:")
		_ = app.enforcer.RevokeConnectionOverride(g.Subject, slug, connID)
	}
	return app.db.DeleteAccessGrant(g.Subject, g.Object)
}
