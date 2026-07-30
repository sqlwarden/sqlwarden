package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/sqlwarden/internal/dbengine/schema"
	build "github.com/sqlwarden/internal/dbengine/schema/build"
)

var _ schema.SchemaInspector = (*sqliteDriver)(nil)
var _ schema.ScopeDiscoverer = (*sqliteDriver)(nil)

func (d *sqliteDriver) SchemaSpec() schema.SchemaSpec {
	return schema.SchemaSpec{
		Dialect: "sqlite",
		Kinds: []schema.SchemaObjectKind{
			{Kind: "table", Label: "Table", PluralLabel: "Tables", Order: 1, Relational: true, SupportsDiagram: true, Listing: "enumerated"},
			{Kind: "view", Label: "View", PluralLabel: "Views", Order: 2, Relational: true, SupportsDiagram: true, Listing: "enumerated"},
			{Kind: "trigger", Label: "Trigger", PluralLabel: "Triggers", Order: 3, Relational: false, SupportsDiagram: false, Listing: "enumerated"},
		},
	}
}

func (d *sqliteDriver) InspectDirectory(ctx context.Context, opts schema.DirectoryOptions) (*schema.Directory, error) {
	b := build.NewDirectory()
	b.DeclareKind("table")
	b.DeclareKind("view")
	b.DeclareKind("trigger")

	namespaces, err := d.sqliteNamespaces(ctx)
	if err != nil {
		return nil, err
	}
	for _, ns := range namespaces {
		scope := schema.NewScopePath(schema.ScopeSegment{Kind: "database", Name: ns})
		if opts.Root != "" && scope != opts.Root {
			continue
		}
		q := fmt.Sprintf(`SELECT type, name FROM %s.sqlite_master WHERE type IN ('table','view','trigger') AND name NOT LIKE 'sqlite_%%' ORDER BY type, name`, sqliteQuoteIdent(ns))
		rows, err := d.db.QueryContext(ctx, q)
		if err != nil {
			return nil, fmt.Errorf("sqlite: catalog objects: %w", err)
		}
		for rows.Next() {
			var typ, name string
			if err := rows.Scan(&typ, &name); err != nil {
				rows.Close()
				return nil, fmt.Errorf("sqlite: catalog objects scan: %w", err)
			}
			b.AddRef(scope, typ, name)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("sqlite: catalog objects rows: %w", err)
		}
		rows.Close()
	}

	root := opts.Root
	if root == "" {
		root = d.defaultScope
	}
	if root != "" {
		return b.Build("", "sqlite", root), nil
	}
	return b.Build("", "sqlite", schema.NewScopePath(schema.ScopeSegment{Kind: "database", Name: "main"})), nil
}

func (d *sqliteDriver) DiscoverScopes(ctx context.Context, request schema.ScopeDiscoveryRequest) (*schema.ScopeDiscovery, error) {
	names, err := d.sqliteNamespaces(ctx)
	if err != nil {
		return nil, err
	}
	result := &schema.ScopeDiscovery{Scopes: make([]schema.ScopePath, 0, len(names))}
	for _, name := range names {
		path := schema.NewScopePath(schema.ScopeSegment{Kind: "database", Name: name})
		result.Scopes = append(result.Scopes, path)
		if name == "main" {
			result.Current = path
		}
	}
	return result, nil
}

func (d *sqliteDriver) InspectObjects(ctx context.Context, refs []schema.ObjectRef) ([]schema.Object, error) {
	allowed, err := d.sqliteNamespaceSet(ctx)
	if err != nil {
		return nil, err
	}
	b := build.NewRelational()
	var triggerRefs []schema.ObjectRef
	for _, ref := range refs {
		if !allowed[ref.Scope.Name("database")] {
			continue
		}
		switch ref.Kind {
		case "table", "view":
			if err := d.inspectSQLiteRelational(ctx, b, ref); err != nil {
				return nil, err
			}
		case "trigger":
			triggerRefs = append(triggerRefs, ref)
		}
	}
	out := b.Build()
	if err := d.attachSQLiteDefinitions(ctx, out); err != nil {
		return nil, err
	}
	if len(triggerRefs) > 0 {
		triggers, err := d.inspectSQLiteTriggers(ctx, triggerRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, triggers...)
	}
	return out, nil
}

// attachSQLiteDefinitions appends the stored CREATE statement from sqlite_master
// as a "source" descriptor: tables get their DDL, views get their definition.
func (d *sqliteDriver) attachSQLiteDefinitions(ctx context.Context, objs []schema.Object) error {
	for i := range objs {
		var typ, title string
		switch objs[i].Ref.Kind {
		case "table":
			typ, title = "table", "DDL"
		case "view":
			typ, title = "view", "Definition"
		default:
			continue
		}
		var ddl sql.NullString
		// SQLite cannot bind identifiers; sqliteQuoteIdent escapes the namespace.
		// codeql[go/sql-injection]
		q := fmt.Sprintf(`SELECT sql FROM %s.sqlite_master WHERE type = ? AND name = ?`, sqliteQuoteIdent(objs[i].Ref.Scope.Name("database")))
		if err := d.db.QueryRowContext(ctx, q, typ, objs[i].Ref.Name).Scan(&ddl); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return fmt.Errorf("sqlite: object definition: %w", err)
		}
		if ddl.Valid && ddl.String != "" {
			objs[i].Descriptors = append(objs[i].Descriptors, schema.Descriptor{
				Kind:   "source",
				Title:  title,
				Source: &schema.Source{Language: "sql", Body: ddl.String},
			})
		}
	}
	return nil
}

func (d *sqliteDriver) sqliteNamespaces(ctx context.Context) ([]string, error) {
	rows, err := d.db.QueryContext(ctx, `PRAGMA database_list`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: database list: %w", err)
	}
	defer rows.Close()

	var namespaces []string
	for rows.Next() {
		var seq int
		var name, file string
		if err := rows.Scan(&seq, &name, &file); err != nil {
			return nil, fmt.Errorf("sqlite: database list scan: %w", err)
		}
		namespaces = append(namespaces, name)
	}
	return namespaces, rows.Err()
}

func (d *sqliteDriver) sqliteNamespaceSet(ctx context.Context) (map[string]bool, error) {
	namespaces, err := d.sqliteNamespaces(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(namespaces))
	for _, ns := range namespaces {
		out[ns] = true
	}
	return out, nil
}

func (d *sqliteDriver) inspectSQLiteRelational(ctx context.Context, b *build.RelationalBuilder, ref schema.ObjectRef) error {
	b.Ensure(ref)

	tableArg := sqliteQuoteIdent(ref.Name)
	prefix := sqliteQuoteIdent(ref.Scope.Name("database"))

	// SQLite cannot bind PRAGMA identifiers; both identifiers are escaped above.
	// codeql[go/sql-injection]
	colQ := fmt.Sprintf(`PRAGMA %s.table_xinfo(%s)`, prefix, tableArg)
	rows, err := d.db.QueryContext(ctx, colQ)
	if err != nil {
		return fmt.Errorf("sqlite: object columns: %w", err)
	}
	for rows.Next() {
		var cid, notNull, pk, hidden int
		var name, dtype string
		var def sql.NullString
		if err := rows.Scan(&cid, &name, &dtype, &notNull, &def, &pk, &hidden); err != nil {
			rows.Close()
			return fmt.Errorf("sqlite: object columns scan: %w", err)
		}
		if hidden != 0 {
			continue
		}
		col := schema.Column{Name: name, DataType: dtype, Nullable: notNull == 0 && pk == 0, Ordinal: cid + 1}
		if def.Valid {
			v := def.String
			col.Default = &v
		}
		b.AddColumn(ref, col)
		if pk > 0 {
			b.AddPrimaryKeyColumn(ref, name)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("sqlite: object columns rows: %w", err)
	}
	rows.Close()

	// codeql[go/sql-injection]
	fkQ := fmt.Sprintf(`PRAGMA %s.foreign_key_list(%s)`, prefix, tableArg)
	fkRows, err := d.db.QueryContext(ctx, fkQ)
	if err != nil {
		return fmt.Errorf("sqlite: object fk: %w", err)
	}
	for fkRows.Next() {
		var id, seq int
		var refTbl, fromCol, toCol, onUpdate, onDelete, match string
		if err := fkRows.Scan(&id, &seq, &refTbl, &fromCol, &toCol, &onUpdate, &onDelete, &match); err != nil {
			fkRows.Close()
			return fmt.Errorf("sqlite: object fk scan: %w", err)
		}
		b.AddForeignKeyColumn(ref, fmt.Sprintf("fk_%d", id), fromCol,
			schema.ObjectRef{Scope: ref.Scope, Kind: "table", Name: refTbl}, toCol)
	}
	if err := fkRows.Err(); err != nil {
		fkRows.Close()
		return fmt.Errorf("sqlite: object fk rows: %w", err)
	}
	fkRows.Close()

	// codeql[go/sql-injection]
	idxQ := fmt.Sprintf(`PRAGMA %s.index_list(%s)`, prefix, tableArg)
	idxRows, err := d.db.QueryContext(ctx, idxQ)
	if err != nil {
		return fmt.Errorf("sqlite: object indexes: %w", err)
	}
	var indexes []schema.SecondaryIndex
	for idxRows.Next() {
		var seq, partial int
		var name, origin string
		var unique int
		if err := idxRows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			idxRows.Close()
			return fmt.Errorf("sqlite: object index scan: %w", err)
		}
		indexes = append(indexes, schema.SecondaryIndex{Name: name, Unique: unique == 1})
	}
	if err := idxRows.Err(); err != nil {
		idxRows.Close()
		return fmt.Errorf("sqlite: object index rows: %w", err)
	}
	idxRows.Close()
	for _, ix := range indexes {
		columns, err := d.sqliteIndexColumns(ctx, ref.Scope.Name("database"), ix.Name)
		if err != nil {
			return err
		}
		ix.Columns = columns
		b.AddIndex(ref, ix)
	}

	return nil
}

func (d *sqliteDriver) sqliteIndexColumns(ctx context.Context, _ string, indexName string) ([]string, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT name FROM pragma_index_info(?) ORDER BY seqno`, indexName)
	if err != nil {
		return nil, fmt.Errorf("sqlite: object index columns: %w", err)
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("sqlite: object index columns scan: %w", err)
		}
		columns = append(columns, name)
	}
	return columns, rows.Err()
}

func (d *sqliteDriver) inspectSQLiteTriggers(ctx context.Context, refs []schema.ObjectRef) ([]schema.Object, error) {
	var out []schema.Object
	for _, ref := range refs {
		// SQLite cannot bind identifiers; sqliteQuoteIdent escapes the namespace.
		// codeql[go/sql-injection]
		q := fmt.Sprintf(`SELECT tbl_name, sql FROM %s.sqlite_master WHERE type = 'trigger' AND name = ?`, sqliteQuoteIdent(ref.Scope.Name("database")))
		row := d.db.QueryRowContext(ctx, q, ref.Name)
		var tableName string
		var definition sql.NullString
		if err := row.Scan(&tableName, &definition); err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			return nil, fmt.Errorf("sqlite: trigger detail: %w", err)
		}
		obj := schema.Object{
			Ref: ref,
			Descriptors: []schema.Descriptor{
				{Kind: "fields", Title: "Trigger", Fields: []schema.Field{{Name: "Table", Value: tableName}}},
			},
		}
		if definition.Valid && definition.String != "" {
			obj.Descriptors = append(obj.Descriptors, schema.Descriptor{
				Kind:   "source",
				Title:  "Definition",
				Source: &schema.Source{Language: "sql", Body: definition.String},
			})
		}
		out = append(out, obj)
	}
	return out, nil
}

func sqliteQuoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
