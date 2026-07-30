package schema

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/sqlwarden/internal/database"
	schemameta "github.com/sqlwarden/internal/dbengine/schema"
	"github.com/uptrace/bun"
)

const (
	SnapshotStatusBuilding = "building"
	SnapshotStatusReady    = "ready"
)

type Snapshot struct {
	bun.BaseModel `bun:"table:schema_snapshots"`

	ID            string     `bun:",pk" json:"id"`
	ConnectionID  int64      `bun:",notnull" json:"connection_id"`
	OrgID         *int64     `bun:",nullzero" json:"org_id,omitempty"`
	Dialect       string     `bun:",notnull" json:"dialect"`
	DatabaseName  string     `bun:",notnull" json:"database"`
	Status        string     `bun:",notnull" json:"status"`
	IsActive      bool       `bun:",notnull" json:"is_active"`
	DirectoryData []byte     `bun:",notnull" json:"-"`
	GeneratedAt   time.Time  `bun:",notnull" json:"generated_at"`
	CreatedAt     time.Time  `bun:",notnull" json:"created_at"`
	CompletedAt   *time.Time `bun:",nullzero" json:"completed_at,omitempty"`
}

type SnapshotStatus struct {
	ID           string     `json:"id,omitempty"`
	ConnectionID int64      `json:"connection_id"`
	Status       string     `json:"status"`
	GeneratedAt  *time.Time `json:"generated_at,omitempty"`
}

type snapshotObject struct {
	bun.BaseModel `bun:"table:schema_snapshot_objects"`

	SnapshotID string `bun:",pk"`
	Scope      string `bun:",pk"`
	Kind       string `bun:",pk"`
	Name       string `bun:",pk"`
	ObjectData []byte `bun:",notnull"`
}

type snapshotRelationship struct {
	bun.BaseModel `bun:"table:schema_snapshot_relationships"`

	SnapshotID       string `bun:",pk"`
	Scope            string `bun:",pk"`
	RelationshipData []byte `bun:",notnull"`
}

// SnapshotStore persists immutable schema generations. The active generation
// is switched only after all object and relationship rows have been written.
type SnapshotStore struct {
	db *database.DB
}

func NewSnapshotStore(db *database.DB) *SnapshotStore {
	return &SnapshotStore{db: db}
}

func (s *SnapshotStore) Begin(ctx context.Context, connectionID int64, orgID *int64, directory *schemameta.Directory) (Snapshot, error) {
	if directory == nil {
		return Snapshot{}, errors.New("schema snapshot directory is required")
	}
	data, err := encodeSnapshotValue(directory)
	if err != nil {
		return Snapshot{}, err
	}
	generatedAt := directory.GeneratedAt
	if generatedAt.IsZero() {
		generatedAt = time.Now()
	}
	snapshot := Snapshot{
		ID:            database.NewID(),
		ConnectionID:  connectionID,
		OrgID:         orgID,
		Dialect:       directory.Engine,
		DatabaseName:  directory.DefaultScope.Name("database"),
		Status:        SnapshotStatusBuilding,
		DirectoryData: data,
		GeneratedAt:   generatedAt,
		CreatedAt:     time.Now(),
	}
	_, err = s.db.NewInsert().Model(&snapshot).Exec(ctx)
	return snapshot, err
}

func (s *SnapshotStore) PutObjects(ctx context.Context, snapshotID string, objects []schemameta.Object) error {
	if len(objects) == 0 {
		return nil
	}
	rows := make([]snapshotObject, 0, len(objects))
	for _, object := range objects {
		data, err := encodeSnapshotValue(object)
		if err != nil {
			return err
		}
		rows = append(rows, snapshotObject{
			SnapshotID: snapshotID,
			Scope:      string(object.Ref.Scope),
			Kind:       object.Ref.Kind,
			Name:       object.Ref.Name,
			ObjectData: data,
		})
	}
	_, err := s.db.NewInsert().Model(&rows).Exec(ctx)
	return err
}

func (s *SnapshotStore) PutRelationship(ctx context.Context, snapshotID string, graph *schemameta.RelationshipGraph) error {
	if graph == nil {
		return nil
	}
	data, err := encodeSnapshotValue(graph)
	if err != nil {
		return err
	}
	row := snapshotRelationship{
		SnapshotID:       snapshotID,
		Scope:            string(graph.Scope),
		RelationshipData: data,
	}
	_, err = s.db.NewInsert().Model(&row).Exec(ctx)
	return err
}

// Publish atomically activates a completed generation, retains the immediately
// previous ready generation, and refuses publication when policy was disabled
// while inspection was running.
func (s *SnapshotStore) Publish(ctx context.Context, snapshotID string) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var snapshot Snapshot
		if err := tx.NewSelect().Model(&snapshot).Where("id = ?", snapshotID).Scan(ctx); err != nil {
			return err
		}
		if snapshot.Status != SnapshotStatusBuilding {
			return fmt.Errorf("schema snapshot %s is not publishable from status %q", snapshotID, snapshot.Status)
		}
		enabled, err := snapshotsEnabledWithExecutor(ctx, tx, snapshot.ConnectionID)
		if err != nil {
			return err
		}
		if !enabled {
			return ErrSnapshotsDisabled
		}

		now := time.Now()
		if _, err := tx.NewUpdate().Model((*Snapshot)(nil)).
			Set("is_active = ?", false).
			Where("connection_id = ? AND is_active = ?", snapshot.ConnectionID, true).
			Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewUpdate().Model((*Snapshot)(nil)).
			Set("status = ?", SnapshotStatusReady).
			Set("is_active = ?", true).
			Set("completed_at = ?", now).
			Where("id = ? AND status = ?", snapshotID, SnapshotStatusBuilding).
			Exec(ctx); err != nil {
			return err
		}

		var retained []string
		if err := tx.NewSelect().Model((*Snapshot)(nil)).
			Column("id").
			Where("connection_id = ? AND status = ?", snapshot.ConnectionID, SnapshotStatusReady).
			OrderExpr("completed_at DESC, created_at DESC").
			Limit(2).
			Scan(ctx, &retained); err != nil {
			return err
		}
		q := tx.NewDelete().Model((*Snapshot)(nil)).
			Where("connection_id = ?", snapshot.ConnectionID).
			Where("status = ?", SnapshotStatusReady)
		if len(retained) > 0 {
			q = q.Where("id NOT IN (?)", bun.List(retained))
		}
		_, err = q.Exec(ctx)
		return err
	})
}

func (s *SnapshotStore) Abort(ctx context.Context, snapshotID string) error {
	_, err := s.db.NewDelete().Model((*Snapshot)(nil)).
		Where("id = ? AND status = ?", snapshotID, SnapshotStatusBuilding).
		Exec(ctx)
	return err
}

func (s *SnapshotStore) Active(ctx context.Context, connectionID int64) (Snapshot, *schemameta.Directory, bool, error) {
	var snapshot Snapshot
	err := s.db.NewSelect().Model(&snapshot).
		Where("connection_id = ? AND is_active = ?", connectionID, true).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return Snapshot{}, nil, false, nil
	}
	if err != nil {
		return Snapshot{}, nil, false, err
	}
	var directory schemameta.Directory
	if err := decodeSnapshotValue(snapshot.DirectoryData, &directory); err != nil {
		return Snapshot{}, nil, false, err
	}
	return snapshot, &directory, true, nil
}

func (s *SnapshotStore) Objects(ctx context.Context, snapshotID string, refs []schemameta.ObjectRef) ([]schemameta.Object, error) {
	out := make([]schemameta.Object, 0, len(refs))
	for _, ref := range refs {
		var row snapshotObject
		err := s.db.NewSelect().Model(&row).
			Where("snapshot_id = ? AND scope = ? AND kind = ? AND name = ?", snapshotID, string(ref.Scope), ref.Kind, ref.Name).
			Scan(ctx)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var object schemameta.Object
		if err := decodeSnapshotValue(row.ObjectData, &object); err != nil {
			return nil, err
		}
		out = append(out, object)
	}
	return out, nil
}

// AllObjects returns every object in an immutable snapshot in stable order.
// Completion uses this bulk path to prepare a dialect-native completion model once.
func (s *SnapshotStore) AllObjects(ctx context.Context, snapshotID string) ([]schemameta.Object, error) {
	var rows []snapshotObject
	if err := s.db.NewSelect().Model(&rows).
		Where("snapshot_id = ?", snapshotID).
		OrderExpr("scope ASC, kind ASC, name ASC").
		Scan(ctx); err != nil {
		return nil, err
	}
	objects := make([]schemameta.Object, 0, len(rows))
	for _, row := range rows {
		var object schemameta.Object
		if err := decodeSnapshotValue(row.ObjectData, &object); err != nil {
			return nil, err
		}
		objects = append(objects, object)
	}
	return objects, nil
}

func (s *SnapshotStore) Relationship(ctx context.Context, snapshotID string, scope schemameta.ScopePath) (*schemameta.RelationshipGraph, bool, error) {
	var row snapshotRelationship
	err := s.db.NewSelect().Model(&row).
		Where("snapshot_id = ? AND scope = ?", snapshotID, string(scope)).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var graph schemameta.RelationshipGraph
	if err := decodeSnapshotValue(row.RelationshipData, &graph); err != nil {
		return nil, false, err
	}
	return &graph, true, nil
}

func (s *SnapshotStore) PurgeConnection(ctx context.Context, connectionID int64) error {
	_, err := s.db.NewDelete().Model((*Snapshot)(nil)).Where("connection_id = ?", connectionID).Exec(ctx)
	return err
}

func (s *SnapshotStore) PurgeOrganization(ctx context.Context, orgID int64) error {
	_, err := s.db.NewDelete().Model((*Snapshot)(nil)).Where("org_id = ?", orgID).Exec(ctx)
	return err
}

var ErrSnapshotsDisabled = errors.New("schema snapshots are disabled")

func snapshotsEnabledWithExecutor(ctx context.Context, exec bun.IDB, connectionID int64) (bool, error) {
	var row struct {
		Policy     string `bun:"schema_snapshot_policy"`
		OrgEnabled *bool  `bun:"schema_snapshots_enabled"`
	}
	err := exec.NewSelect().
		TableExpr("connections AS c").
		ColumnExpr("c.schema_snapshot_policy").
		ColumnExpr("o.schema_snapshots_enabled").
		Join("JOIN workspaces AS w ON w.id = c.workspace_id").
		Join("LEFT JOIN organizations AS o ON o.id = w.org_id").
		Where("c.id = ?", connectionID).
		Scan(ctx, &row)
	if err != nil {
		return false, err
	}
	if row.Policy == database.SchemaSnapshotPolicyDisabled {
		return false, nil
	}
	return row.OrgEnabled == nil || *row.OrgEnabled, nil
}

func encodeSnapshotValue(value any) ([]byte, error) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		_ = writer.Close()
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return compressed.Bytes(), nil
}

func decodeSnapshotValue(data []byte, target any) error {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("open schema snapshot: %w", err)
	}
	defer reader.Close()
	limited := io.LimitReader(reader, 64<<20)
	if err := json.NewDecoder(limited).Decode(target); err != nil {
		return fmt.Errorf("decode schema snapshot: %w", err)
	}
	return nil
}
