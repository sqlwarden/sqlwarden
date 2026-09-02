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
	SnapshotID  string    `json:"snapshot_id"`
	GeneratedAt time.Time `json:"generated_at"`
	Objects     int       `json:"objects"`
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
	runtimeSettings, err := app.runtimeSettingsService().effectiveForOrg(ctx, orgID)
	if err != nil {
		app.logger.WarnContext(ctx, "runtime settings lookup failed", "connection_id", conn.ID, "error", err)
		return
	}
	_, directory, found, err := app.schemaSnapshots.Active(ctx, conn.ID)
	if err != nil {
		app.logger.WarnContext(ctx, "schema snapshot freshness check failed", "connection_id", conn.ID, "error", err)
		return
	}
	if found && time.Since(directory.GeneratedAt) < runtimeSettings.SchemaSnapshotFreshness {
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
	return app.syncSchemaSnapshot(ctx, input.ConnectionID)
}

// syncSchemaSnapshot builds and atomically publishes one complete metadata
// generation. It is shared by automatic jobs and user-requested synchronous
// refreshes so both paths retain identical inspection and cache behavior.
func (app *application) syncSchemaSnapshot(ctx context.Context, connectionID int64) (schemaSyncOutput, error) {
	startedAt := time.Now()
	enabled, err := app.db.SchemaSnapshotsEnabled(ctx, connectionID)
	if err != nil {
		return schemaSyncOutput{}, err
	}
	if !enabled {
		return schemaSyncOutput{}, jobs.Permanent("schema_snapshots_disabled", "Schema snapshots are disabled.")
	}
	conn, found, err := app.db.GetConnection(ctx, connectionID)
	if err != nil {
		return schemaSyncOutput{}, err
	}
	if !found {
		return schemaSyncOutput{}, jobs.Permanent("connection_not_found", "Connection was not found.")
	}
	ws, found, err := app.db.GetWorkspace(ctx, conn.WorkspaceID)
	if err != nil {
		return schemaSyncOutput{}, err
	}
	if !found {
		return schemaSyncOutput{}, jobs.Permanent("workspace_not_found", "Workspace was not found.")
	}

	driver, err := app.openTargetDriver(ctx, conn, ws)
	if err != nil {
		return schemaSyncOutput{}, err
	}
	defer driver.Close()

	inspector, ok := driver.(metadata.SchemaInspector)
	if !ok {
		return schemaSyncOutput{}, jobs.Permanent("schema_sync_unsupported", "Schema inspection is not supported for this driver.")
	}
	directoryStartedAt := time.Now()
	directory, err := inspector.InspectDirectory(ctx, metadata.DirectoryOptions{Root: conn.DefaultScope})
	if err != nil {
		return schemaSyncOutput{}, jobs.Retryable("schema_directory_failed", "Could not inspect the schema directory.")
	}
	directoryElapsed := time.Since(directoryStartedAt)
	directory.Connection = strconv.FormatInt(conn.ID, 10)
	// Ordering by operation start prevents a slower, older concurrent refresh
	// from replacing a newer generation that finishes first.
	directory.GeneratedAt = startedAt

	snapshot, err := app.schemaSnapshots.Begin(ctx, conn.ID, ws.OrgID, directory)
	if err != nil {
		return schemaSyncOutput{}, err
	}
	published := false
	defer func() {
		if !published {
			_ = app.schemaSnapshots.Abort(context.Background(), snapshot.ID)
		}
	}()

	refs := directoryObjectRefs(directory)
	objectsStartedAt := time.Now()
	objectCount, err := app.inspectAndStoreObjects(ctx, inspector, snapshot.ID, refs)
	if err != nil {
		return schemaSyncOutput{}, err
	}
	objectsElapsed := time.Since(objectsStartedAt)

	relationshipsStartedAt := time.Now()
	if relationshipInspector, ok := driver.(metadata.RelationshipInspector); ok {
		for _, scope := range directoryObjectScopes(directory) {
			graph, inspectErr := relationshipInspector.InspectRelationshipsInScope(ctx, scope)
			if inspectErr != nil {
				return schemaSyncOutput{}, jobs.Retryable("schema_relationships_failed", "Could not inspect schema relationships.")
			}
			if err := app.schemaSnapshots.PutRelationship(ctx, snapshot.ID, graph); err != nil {
				return schemaSyncOutput{}, err
			}
		}
	}
	relationshipsElapsed := time.Since(relationshipsStartedAt)
	publishStartedAt := time.Now()

	if err := app.schemaSnapshots.Publish(ctx, snapshot.ID); err != nil {
		if errors.Is(err, schemaapp.ErrSnapshotSuperseded) {
			active, directory, found, activeErr := app.schemaSnapshots.Active(ctx, conn.ID)
			if activeErr != nil {
				return schemaSyncOutput{}, activeErr
			}
			if found {
				return schemaSyncOutput{SnapshotID: active.ID, GeneratedAt: directory.GeneratedAt}, nil
			}
			return schemaSyncOutput{}, err
		}
		if errors.Is(err, schemaapp.ErrSnapshotsDisabled) {
			return schemaSyncOutput{}, jobs.Permanent("schema_snapshots_disabled", "Schema snapshots were disabled during synchronization.")
		}
		return schemaSyncOutput{}, err
	}
	published = true
	app.schemaService.RefreshConnection(strconv.FormatInt(conn.ID, 10))
	app.completionService.InvalidateConnection(strconv.FormatInt(conn.ID, 10))
	app.logger.InfoContext(ctx, "schema snapshot published",
		"connection_id", conn.ID,
		"engine", directory.Engine,
		"scopes", len(directoryObjectScopes(directory)),
		"objects", objectCount,
		"directory_ms", directoryElapsed.Milliseconds(),
		"objects_ms", objectsElapsed.Milliseconds(),
		"relationships_ms", relationshipsElapsed.Milliseconds(),
		"publish_ms", time.Since(publishStartedAt).Milliseconds(),
		"total_ms", time.Since(startedAt).Milliseconds(),
	)
	return schemaSyncOutput{SnapshotID: snapshot.ID, GeneratedAt: directory.GeneratedAt, Objects: objectCount}, nil
}

// inspectAndStoreObjects walks refs in schemaObjectBatchSize batches, overlapping
// each batch's target-database inspection with the previous batch's write to the
// snapshot store. Inspection stays strictly sequential against the single target
// connection; only the local PutObjects write runs concurrently with the next
// InspectObjects call, so a slow metadata store no longer stalls target reads.
func (app *application) inspectAndStoreObjects(ctx context.Context, inspector metadata.SchemaInspector, snapshotID string, refs []metadata.ObjectRef) (int, error) {
	type batch struct {
		objects []metadata.Object
		err     error
	}
	// Depth 1: the inspector may run at most one batch ahead of the writer.
	batches := make(chan batch, 1)
	inspectCtx, cancelInspect := context.WithCancel(ctx)
	defer cancelInspect()

	go func() {
		defer close(batches)
		for start := 0; start < len(refs); start += schemaObjectBatchSize {
			end := min(start+schemaObjectBatchSize, len(refs))
			objects, inspectErr := inspector.InspectObjects(inspectCtx, refs[start:end])
			select {
			case batches <- batch{objects: objects, err: inspectErr}:
			case <-inspectCtx.Done():
				return
			}
			if inspectErr != nil {
				return
			}
		}
	}()

	objectCount := 0
	for b := range batches {
		if b.err != nil {
			return 0, jobs.Retryable("schema_objects_failed", "Could not inspect schema object details.")
		}
		if err := app.schemaSnapshots.PutObjects(ctx, snapshotID, b.objects); err != nil {
			return 0, err
		}
		objectCount += len(b.objects)
	}
	return objectCount, nil
}

// openTargetDriver connects a fresh engine driver to conn's target database for a
// one-off inspection outside the pooled live-session path (schema sync, lazy
// definition fetch). The caller owns the returned driver and must Close it. Error
// results are jobs.CodedError values so both the job runner and HTTP callers can
// classify them.
func (app *application) openTargetDriver(ctx context.Context, conn database.Connection, ws database.Workspace) (engine.Driver, error) {
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
	settings, err := app.effectiveRuntimeSettingsForWorkspace(ctx, ws)
	if err != nil {
		return nil, err
	}
	if err := driver.Connect(ctx, app.driverConnectionConfig(conn.Driver, plainDSN, settings, conn.DefaultScope)); err != nil {
		return nil, jobs.Retryable("schema_sync_connect_failed", "Could not connect to the target database.")
	}
	return driver, nil
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
