package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/engine"
	metadata "github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/jobs"
	schemaapp "github.com/sqlwarden/internal/schema"
)

const schemaObjectBatchSize = 100

type schemaSyncInput struct {
	ConnectionID int64 `json:"connection_id"`
}

type schemaSyncOutput struct {
	SnapshotID string `json:"snapshot_id"`
	Objects    int    `json:"objects"`
}

func schemaSyncSingletonKey(connectionID int64) string {
	return "schema-sync:" + strconv.FormatInt(connectionID, 10)
}

func (app *application) enqueueSchemaSync(ctx context.Context, connectionID int64, orgID *int64) (jobs.Record, bool, error) {
	enabled, err := app.db.SchemaSnapshotsEnabled(ctx, connectionID)
	if err != nil || !enabled {
		return jobs.Record{}, false, err
	}
	return app.workspaceJobStore().EnqueueSingleton(ctx, jobs.EnqueueInput{
		Type:         jobs.TypeSchemaSync,
		SingletonKey: schemaSyncSingletonKey(connectionID),
		Visibility:   jobs.VisibilityInternal,
		OrgID:        orgID,
		Priority:     jobs.PriorityLow,
		MaxAttempts:  3,
		Input:        schemaSyncInput{ConnectionID: connectionID},
	})
}

func (app *application) maybeEnqueueSchemaSync(ctx context.Context, conn database.Connection, orgID *int64) {
	if app.schemaSnapshots == nil {
		return
	}
	enabled, err := app.db.SchemaSnapshotsEnabled(ctx, conn.ID)
	if err != nil || !enabled {
		return
	}
	_, directory, found, err := app.schemaSnapshots.Active(ctx, conn.ID)
	if err != nil {
		app.logger.WarnContext(ctx, "schema snapshot freshness check failed", "connection_id", conn.ID, "error", err)
		return
	}
	if found && time.Since(directory.GeneratedAt) < app.config.Schema.SnapshotFreshness {
		return
	}
	if _, _, err := app.enqueueSchemaSync(ctx, conn.ID, orgID); err != nil && !errors.Is(err, jobs.ErrActiveExists) {
		app.logger.WarnContext(ctx, "schema snapshot enqueue failed", "connection_id", conn.ID, "error", err)
	}
}

func (app *application) handleSchemaSyncJob(ctx context.Context, runtime jobs.Runtime) (output any, err error) {
	var input schemaSyncInput
	if err := json.Unmarshal([]byte(runtime.Job.InputJSON), &input); err != nil || input.ConnectionID == 0 {
		return nil, jobs.Permanent("invalid_schema_sync_input", "Schema synchronization input is invalid.")
	}

	enabled, err := app.db.SchemaSnapshotsEnabled(ctx, input.ConnectionID)
	if err != nil {
		return nil, err
	}
	if !enabled {
		return nil, jobs.Permanent("schema_snapshots_disabled", "Schema snapshots are disabled.")
	}
	conn, found, err := app.db.GetConnection(ctx, input.ConnectionID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, jobs.Permanent("connection_not_found", "Connection was not found.")
	}
	ws, found, err := app.db.GetWorkspace(ctx, conn.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, jobs.Permanent("workspace_not_found", "Workspace was not found.")
	}

	plainDSN, err := app.keyring.Decrypt(conn.DSNEncrypted)
	if err != nil {
		return nil, err
	}
	if err := app.validateTargetConnection(conn.Driver, plainDSN); err != nil {
		return nil, jobs.Permanent("schema_sync_target_blocked", "The target database is blocked by policy.")
	}
	driver, err := engine.New(conn.Driver)
	if err != nil {
		return nil, jobs.Permanent("schema_sync_driver_unavailable", "The target driver is unavailable.")
	}
	if err := driver.Connect(ctx, app.driverConnectionConfig(conn.Driver, plainDSN, conn.DefaultScope)); err != nil {
		return nil, jobs.Retryable("schema_sync_connect_failed", "Could not connect to the target database.")
	}
	defer driver.Close()

	inspector, ok := driver.(metadata.SchemaInspector)
	if !ok {
		return nil, jobs.Permanent("schema_sync_unsupported", "Schema inspection is not supported for this driver.")
	}
	directory, err := inspector.InspectDirectory(ctx, metadata.DirectoryOptions{Root: conn.DefaultScope})
	if err != nil {
		return nil, jobs.Retryable("schema_directory_failed", "Could not inspect the schema directory.")
	}
	directory.Connection = strconv.FormatInt(conn.ID, 10)
	if directory.GeneratedAt.IsZero() {
		directory.GeneratedAt = time.Now()
	}

	snapshot, err := app.schemaSnapshots.Begin(ctx, conn.ID, ws.OrgID, directory)
	if err != nil {
		return nil, err
	}
	published := false
	defer func() {
		if !published {
			_ = app.schemaSnapshots.Abort(context.Background(), snapshot.ID)
		}
	}()

	refs := directoryObjectRefs(directory)
	objectCount := 0
	for start := 0; start < len(refs); start += schemaObjectBatchSize {
		end := start + schemaObjectBatchSize
		if end > len(refs) {
			end = len(refs)
		}
		objects, inspectErr := inspector.InspectObjects(ctx, refs[start:end])
		if inspectErr != nil {
			return nil, jobs.Retryable("schema_objects_failed", "Could not inspect schema object details.")
		}
		if err := app.schemaSnapshots.PutObjects(ctx, snapshot.ID, objects); err != nil {
			return nil, err
		}
		objectCount += len(objects)
	}

	if relationshipInspector, ok := driver.(metadata.RelationshipInspector); ok {
		for _, scope := range directoryObjectScopes(directory) {
			graph, inspectErr := relationshipInspector.InspectRelationshipsInScope(ctx, scope)
			if inspectErr != nil {
				return nil, jobs.Retryable("schema_relationships_failed", "Could not inspect schema relationships.")
			}
			if err := app.schemaSnapshots.PutRelationship(ctx, snapshot.ID, graph); err != nil {
				return nil, err
			}
		}
	}

	if err := app.schemaSnapshots.Publish(ctx, snapshot.ID); err != nil {
		if errors.Is(err, schemaapp.ErrSnapshotsDisabled) {
			return nil, jobs.Permanent("schema_snapshots_disabled", "Schema snapshots were disabled during synchronization.")
		}
		return nil, err
	}
	published = true
	app.schemaService.RefreshConnection(strconv.FormatInt(conn.ID, 10))
	app.completionService.InvalidateConnection(strconv.FormatInt(conn.ID, 10))
	app.logger.InfoContext(ctx, "schema snapshot published",
		"connection_id", conn.ID,
		"engine", directory.Engine,
		"scopes", len(directoryObjectScopes(directory)),
		"objects", objectCount,
	)
	return schemaSyncOutput{SnapshotID: snapshot.ID, Objects: objectCount}, nil
}

func directoryObjectRefs(directory *metadata.Directory) []metadata.ObjectRef {
	if directory == nil {
		return nil
	}
	var refs []metadata.ObjectRef
	walkDirectoryNodes(directory.Roots, func(node metadata.ScopeNode) {
		for _, group := range node.Groups {
			refs = append(refs, group.Objects...)
		}
	})
	return refs
}

func directoryObjectScopes(directory *metadata.Directory) []metadata.ScopePath {
	if directory == nil {
		return nil
	}
	var scopes []metadata.ScopePath
	walkDirectoryNodes(directory.Roots, func(node metadata.ScopeNode) {
		if len(node.Groups) > 0 {
			scopes = append(scopes, node.Path)
		}
	})
	return scopes
}

func walkDirectoryNodes(nodes []metadata.ScopeNode, visit func(metadata.ScopeNode)) {
	for _, node := range nodes {
		visit(node)
		walkDirectoryNodes(node.Children, visit)
	}
}

func (app *application) disableConnectionSnapshots(ctx context.Context, connectionID int64) error {
	if err := app.workspaceJobStore().RequestCancelSingleton(ctx, schemaSyncSingletonKey(connectionID)); err != nil {
		return fmt.Errorf("cancel schema sync: %w", err)
	}
	if app.schemaSnapshots != nil {
		if err := app.schemaSnapshots.PurgeConnection(ctx, connectionID); err != nil {
			return fmt.Errorf("purge schema snapshots: %w", err)
		}
	}
	app.schemaService.RefreshConnection(strconv.FormatInt(connectionID, 10))
	app.completionService.InvalidateConnection(strconv.FormatInt(connectionID, 10))
	return nil
}

func (app *application) disableOrganizationSnapshots(ctx context.Context, orgID int64) error {
	connections, err := app.db.ListOrgConnections(ctx, orgID)
	if err != nil {
		return err
	}
	for _, conn := range connections {
		if err := app.workspaceJobStore().RequestCancelSingleton(ctx, schemaSyncSingletonKey(conn.ID)); err != nil {
			return err
		}
		app.schemaService.RefreshConnection(strconv.FormatInt(conn.ID, 10))
		app.completionService.InvalidateConnection(strconv.FormatInt(conn.ID, 10))
	}
	if app.schemaSnapshots != nil {
		return app.schemaSnapshots.PurgeOrganization(ctx, orgID)
	}
	return nil
}
