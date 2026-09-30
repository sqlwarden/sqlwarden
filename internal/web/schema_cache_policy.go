package web

import (
	"context"
	"fmt"
	"strconv"
)

func (app *application) purgeConnectionSchemaCache(ctx context.Context, connectionID int64) error {
	if err := app.db.DeleteSchemaCache(ctx, connectionID); err != nil {
		return fmt.Errorf("purge schema cache: %w", err)
	}
	app.schemaNavigator.ForgetConnection(connectionID)
	app.completionService.InvalidateConnection(strconv.FormatInt(connectionID, 10))
	return nil
}

func (app *application) disableOrganizationSnapshots(ctx context.Context, orgID int64) error {
	connections, err := app.db.ListOrgConnections(ctx, orgID)
	if err != nil {
		return err
	}
	for _, conn := range connections {
		if err := app.purgeConnectionSchemaCache(ctx, conn.ID); err != nil {
			return err
		}
	}
	return nil
}
