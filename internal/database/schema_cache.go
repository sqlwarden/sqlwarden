package database

import (
	"context"
	"strings"
	"time"

	"github.com/uptrace/bun"
)

type SchemaListing struct {
	bun.BaseModel `bun:"table:schema_nodes"`

	ConnectionID int64     `bun:",pk"`
	ParentPath   string    `bun:",pk"`
	Folder       string    `bun:",pk"`
	ChildrenData []byte    `bun:",notnull"`
	FetchedAt    time.Time `bun:",notnull"`
}

type SchemaObject struct {
	bun.BaseModel `bun:"table:schema_objects"`

	ConnectionID int64     `bun:",pk"`
	Scope        string    `bun:",pk"`
	Kind         string    `bun:",pk"`
	Name         string    `bun:",pk"`
	ObjectData   []byte    `bun:",notnull"`
	FetchedAt    time.Time `bun:",notnull"`
}

type SchemaRelationship struct {
	bun.BaseModel `bun:"table:schema_relationships"`

	ConnectionID int64     `bun:",pk"`
	Scope        string    `bun:",pk"`
	Data         []byte    `bun:",notnull"`
	FetchedAt    time.Time `bun:",notnull"`
}

type SchemaObjectKey struct {
	Scope, Kind, Name string
}

// likeSubtreePattern escapes LIKE wildcards in root and matches strict
// descendants; the trailing "/" keeps database=a from matching database=ab.
func likeSubtreePattern(root string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(root)
	return escaped + "/%"
}

func whereWithin(q *bun.SelectQuery, column, root string) *bun.SelectQuery {
	if root == "" {
		return q
	}
	return q.Where("(? = ? OR ? LIKE ? ESCAPE '\\')", bun.Ident(column), root, bun.Ident(column), likeSubtreePattern(root))
}

func deleteWithin(q *bun.DeleteQuery, column, root string) *bun.DeleteQuery {
	if root == "" {
		return q
	}
	return q.Where("(? = ? OR ? LIKE ? ESCAPE '\\')", bun.Ident(column), root, bun.Ident(column), likeSubtreePattern(root))
}

func (db *DB) SchemaListing(ctx context.Context, connID int64, parentPath, folder string) (SchemaListing, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	var rows []SchemaListing
	err := db.NewSelect().Model(&rows).
		Where("connection_id = ?", connID).
		Where("parent_path = ?", parentPath).
		Where("folder = ?", folder).
		Limit(1).
		Scan(ctx)
	if err != nil || len(rows) == 0 {
		return SchemaListing{}, false, err
	}
	return rows[0], true, nil
}

func (db *DB) SchemaListingsWithin(ctx context.Context, connID int64, root string) ([]SchemaListing, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	var rows []SchemaListing
	q := db.NewSelect().Model(&rows).Where("connection_id = ?", connID)
	err := whereWithin(q, "parent_path", root).Scan(ctx)
	return rows, err
}

func (db *DB) UpsertSchemaListings(ctx context.Context, rows []SchemaListing) error {
	if len(rows) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	_, err := db.NewInsert().Model(&rows).
		On("CONFLICT (connection_id, parent_path, folder) DO UPDATE").
		Set("children_data = EXCLUDED.children_data").
		Set("fetched_at = EXCLUDED.fetched_at").
		Exec(ctx)
	return err
}

func (db *DB) DeleteSchemaListingsWithin(ctx context.Context, connID int64, root string) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	q := db.NewDelete().Model((*SchemaListing)(nil)).Where("connection_id = ?", connID)
	_, err := deleteWithin(q, "parent_path", root).Exec(ctx)
	return err
}

func (db *DB) SchemaObjects(ctx context.Context, connID int64, keys []SchemaObjectKey) ([]SchemaObject, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	var rows []SchemaObject
	err := db.NewSelect().Model(&rows).
		Where("connection_id = ?", connID).
		WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			for _, key := range keys {
				q = q.WhereOr("(scope = ? AND kind = ? AND name = ?)", key.Scope, key.Kind, key.Name)
			}
			return q
		}).
		Scan(ctx)
	return rows, err
}

func (db *DB) AllSchemaObjects(ctx context.Context, connID int64) ([]SchemaObject, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	var rows []SchemaObject
	err := db.NewSelect().Model(&rows).Where("connection_id = ?", connID).Scan(ctx)
	return rows, err
}

func (db *DB) UpsertSchemaObjects(ctx context.Context, rows []SchemaObject) error {
	if len(rows) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	_, err := db.NewInsert().Model(&rows).
		On("CONFLICT (connection_id, scope, kind, name) DO UPDATE").
		Set("object_data = EXCLUDED.object_data").
		Set("fetched_at = EXCLUDED.fetched_at").
		Exec(ctx)
	return err
}

// DeleteSchemaObjects removes objects whose scope lies within the given root
// plus the exact keys listed (the root object itself and its ancestors).
func (db *DB) DeleteSchemaObjects(ctx context.Context, connID int64, within string, exact []SchemaObjectKey) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	q := db.NewDelete().Model((*SchemaObject)(nil)).
		Where("connection_id = ?", connID).
		WhereGroup(" AND ", func(q *bun.DeleteQuery) *bun.DeleteQuery {
			if within == "" {
				return q.Where("1 = 1")
			}
			q = q.Where("(scope = ? OR scope LIKE ? ESCAPE '\\')", within, likeSubtreePattern(within))
			for _, key := range exact {
				q = q.WhereOr("(scope = ? AND kind = ? AND name = ?)", key.Scope, key.Kind, key.Name)
			}
			return q
		})
	_, err := q.Exec(ctx)
	return err
}

func (db *DB) SchemaRelationship(ctx context.Context, connID int64, scope string) (SchemaRelationship, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	var rows []SchemaRelationship
	err := db.NewSelect().Model(&rows).
		Where("connection_id = ?", connID).
		Where("scope = ?", scope).
		Limit(1).
		Scan(ctx)
	if err != nil || len(rows) == 0 {
		return SchemaRelationship{}, false, err
	}
	return rows[0], true, nil
}

func (db *DB) UpsertSchemaRelationship(ctx context.Context, row SchemaRelationship) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	_, err := db.NewInsert().Model(&row).
		On("CONFLICT (connection_id, scope) DO UPDATE").
		Set("data = EXCLUDED.data").
		Set("fetched_at = EXCLUDED.fetched_at").
		Exec(ctx)
	return err
}

// DeleteSchemaRelationships removes graphs scoped within root and graphs for
// the exact ancestor scopes, whose edges may reference objects under root.
func (db *DB) DeleteSchemaRelationships(ctx context.Context, connID int64, within string, exact []string) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	q := db.NewDelete().Model((*SchemaRelationship)(nil)).
		Where("connection_id = ?", connID).
		WhereGroup(" AND ", func(q *bun.DeleteQuery) *bun.DeleteQuery {
			if within == "" {
				return q.Where("1 = 1")
			}
			q = q.Where("(scope = ? OR scope LIKE ? ESCAPE '\\')", within, likeSubtreePattern(within))
			if len(exact) > 0 {
				q = q.WhereOr("scope IN (?)", bun.List(exact))
			}
			return q
		})
	_, err := q.Exec(ctx)
	return err
}

func (db *DB) DeleteSchemaCache(ctx context.Context, connID int64) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		for _, model := range []any{(*SchemaListing)(nil), (*SchemaObject)(nil), (*SchemaRelationship)(nil)} {
			if _, err := tx.NewDelete().Model(model).Where("connection_id = ?", connID).Exec(ctx); err != nil {
				return err
			}
		}
		return nil
	})
}
