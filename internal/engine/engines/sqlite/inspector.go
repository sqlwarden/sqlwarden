package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	rqlitesql "github.com/rqlite/sql"

	"github.com/sqlwarden/internal/engine/metadata"
	build "github.com/sqlwarden/internal/engine/metadata/build"
)

var _ metadata.SchemaInspector = (*sqliteDriver)(nil)
var _ metadata.ScopeDiscoverer = (*sqliteDriver)(nil)
var _ metadata.DefinitionInspector = (*sqliteDriver)(nil)

func (d *sqliteDriver) SchemaSpec() metadata.SchemaSpec {
	return metadata.SchemaSpec{
		Dialect: "sqlite",
		Kinds: []metadata.SchemaObjectKind{
			{Kind: "table", Label: "Table", PluralLabel: "Tables", Order: 1, Relational: true, SupportsDiagram: true, Listing: "enumerated"},
			{Kind: "view", Label: "View", PluralLabel: "Views", Order: 2, Relational: true, SupportsDiagram: true, Listing: "enumerated"},
			{Kind: "trigger", Label: "Trigger", PluralLabel: "Triggers", Order: 3, Relational: false, SupportsDiagram: false, Listing: "enumerated"},
		},
	}
}

func (d *sqliteDriver) InspectDirectory(ctx context.Context, opts metadata.DirectoryOptions) (*metadata.Directory, error) {
	b := build.NewDirectory()
	b.DeclareKind("table")
	b.DeclareKind("view")
	b.DeclareKind("trigger")

	namespaces, err := d.sqliteNamespaces(ctx)
	if err != nil {
		return nil, err
	}
	for _, ns := range namespaces {
		scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: ns})
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
	return b.Build("", "sqlite", metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "main"})), nil
}

func (d *sqliteDriver) DiscoverScopes(ctx context.Context, request metadata.ScopeDiscoveryRequest) (*metadata.ScopeDiscovery, error) {
	names, err := d.sqliteNamespaces(ctx)
	if err != nil {
		return nil, err
	}
	result := &metadata.ScopeDiscovery{Scopes: make([]metadata.ScopePath, 0, len(names))}
	for _, name := range names {
		path := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: name})
		result.Scopes = append(result.Scopes, path)
		if name == "main" {
			result.Current = path
		}
	}
	return result, nil
}

func (d *sqliteDriver) InspectObjects(ctx context.Context, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	allowed, err := d.sqliteNamespaceSet(ctx)
	if err != nil {
		return nil, err
	}
	b := build.NewRelational()
	var triggerRefs []metadata.ObjectRef
	descByRef := map[metadata.ObjectRef][]metadata.Descriptor{}
	for _, ref := range refs {
		if !allowed[ref.Scope.Name("database")] {
			continue
		}
		switch ref.Kind {
		case "table", "view":
			descs, err := d.inspectSQLiteRelational(ctx, b, ref)
			if err != nil {
				return nil, err
			}
			if len(descs) > 0 {
				descByRef[ref] = descs
			}
		case "trigger":
			triggerRefs = append(triggerRefs, ref)
		}
	}
	out := b.Build()
	for i := range out {
		if descs, ok := descByRef[out[i].Ref]; ok {
			out[i].Descriptors = append(out[i].Descriptors, descs...)
		}
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

// InspectDefinition returns the stored CREATE statement for a table, view, or
// trigger as a single source descriptor, fetched lazily rather than in bulk
// InspectObjects.
func (d *sqliteDriver) InspectDefinition(ctx context.Context, ref metadata.ObjectRef) (*metadata.Descriptor, error) {
	ns := ref.Scope.Name("database")
	allowed, err := d.sqliteNamespaceSet(ctx)
	if err != nil {
		return nil, err
	}
	if !allowed[ns] {
		return nil, nil
	}
	var typ, title string
	switch ref.Kind {
	case "table":
		typ, title = "table", "DDL"
	case "view":
		typ, title = "view", "Definition"
	case "trigger":
		typ, title = "trigger", "Definition"
	default:
		return nil, nil
	}
	var ddl sql.NullString
	// Identifiers cannot be bound; sqliteQuoteIdent escapes the namespace.
	// codeql[go/sql-injection]
	q := fmt.Sprintf(`SELECT sql FROM %s.sqlite_master WHERE type = ? AND name = ?`, sqliteQuoteIdent(ns))
	if err := d.db.QueryRowContext(ctx, q, typ, ref.Name).Scan(&ddl); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("sqlite: object definition: %w", err)
	}
	if !ddl.Valid || ddl.String == "" {
		return nil, nil
	}
	return &metadata.Descriptor{
		Kind:   "source",
		Title:  title,
		Source: &metadata.Source{Language: "sql", Body: ddl.String},
	}, nil
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

func (d *sqliteDriver) inspectSQLiteRelational(ctx context.Context, b *build.RelationalBuilder, ref metadata.ObjectRef) ([]metadata.Descriptor, error) {
	b.Ensure(ref)

	tableArg := sqliteQuoteIdent(ref.Name)
	prefix := sqliteQuoteIdent(ref.Scope.Name("database"))

	enrich := d.sqliteTableEnrichment(ctx, ref)

	// SQLite cannot bind PRAGMA identifiers; both identifiers are escaped above.
	// codeql[go/sql-injection]
	colQ := fmt.Sprintf(`PRAGMA %s.table_xinfo(%s)`, prefix, tableArg)
	rows, err := d.db.QueryContext(ctx, colQ)
	if err != nil {
		return nil, fmt.Errorf("sqlite: object columns: %w", err)
	}
	var generatedCols []string
	for rows.Next() {
		var cid, notNull, pk, hidden int
		var name, dtype string
		var def sql.NullString
		if err := rows.Scan(&cid, &name, &dtype, &notNull, &def, &pk, &hidden); err != nil {
			rows.Close()
			return nil, fmt.Errorf("sqlite: object columns scan: %w", err)
		}
		// table_xinfo.hidden: 0 normal, 1 truly hidden (skip), 2 VIRTUAL
		// generated, 3 STORED generated (both surfaced as regular columns).
		if hidden == 1 {
			continue
		}
		col := metadata.Column{Name: name, DataType: dtype, Nullable: notNull == 0 && pk == 0, Ordinal: cid + 1}
		if def.Valid {
			v := def.String
			col.Default = &v
		}
		switch hidden {
		case 2:
			sqliteSetColAttr(&col, "generated", "virtual")
			generatedCols = append(generatedCols, name)
		case 3:
			sqliteSetColAttr(&col, "generated", "stored")
			generatedCols = append(generatedCols, name)
		}
		if enrich != nil {
			if c, ok := enrich.collation[name]; ok {
				sqliteSetColAttr(&col, "collation", c)
			}
			if c, ok := enrich.colChecks[name]; ok {
				sqliteSetColAttr(&col, "check", c)
			}
		}
		b.AddColumn(ref, col)
		if pk > 0 {
			b.AddPrimaryKeyColumn(ref, name)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("sqlite: object columns rows: %w", err)
	}
	rows.Close()

	// codeql[go/sql-injection]
	fkQ := fmt.Sprintf(`PRAGMA %s.foreign_key_list(%s)`, prefix, tableArg)
	fkRows, err := d.db.QueryContext(ctx, fkQ)
	if err != nil {
		return nil, fmt.Errorf("sqlite: object fk: %w", err)
	}
	for fkRows.Next() {
		var id, seq int
		var refTbl, fromCol, toCol, onUpdate, onDelete, match string
		if err := fkRows.Scan(&id, &seq, &refTbl, &fromCol, &toCol, &onUpdate, &onDelete, &match); err != nil {
			fkRows.Close()
			return nil, fmt.Errorf("sqlite: object fk scan: %w", err)
		}
		b.AddForeignKeyColumn(ref, fmt.Sprintf("fk_%d", id), fromCol,
			metadata.ObjectRef{Scope: ref.Scope, Kind: "table", Name: refTbl}, toCol)
	}
	if err := fkRows.Err(); err != nil {
		fkRows.Close()
		return nil, fmt.Errorf("sqlite: object fk rows: %w", err)
	}
	fkRows.Close()

	// codeql[go/sql-injection]
	idxQ := fmt.Sprintf(`PRAGMA %s.index_list(%s)`, prefix, tableArg)
	idxRows, err := d.db.QueryContext(ctx, idxQ)
	if err != nil {
		return nil, fmt.Errorf("sqlite: object indexes: %w", err)
	}
	var indexes []metadata.SecondaryIndex
	for idxRows.Next() {
		var seq, partial int
		var name, origin string
		var unique int
		if err := idxRows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			idxRows.Close()
			return nil, fmt.Errorf("sqlite: object index scan: %w", err)
		}
		// Only indexes created explicitly by the user are independently
		// droppable. SQLite owns indexes backing primary/unique constraints.
		if origin != "c" {
			continue
		}
		indexes = append(indexes, metadata.SecondaryIndex{Name: name, Unique: unique == 1})
	}
	if err := idxRows.Err(); err != nil {
		idxRows.Close()
		return nil, fmt.Errorf("sqlite: object index rows: %w", err)
	}
	idxRows.Close()
	for _, ix := range indexes {
		columns, err := d.sqliteIndexColumns(ctx, ix.Name)
		if err != nil {
			return nil, err
		}
		ix.Columns = columns
		b.AddIndex(ref, ix)
	}

	return sqliteConstraintDescriptors(enrich, generatedCols), nil
}

// sqliteIndexColumns returns an index's own key columns in seqno order via
// index_xinfo. Filtering on key == 1 excludes the auxiliary trailing entries
// (rowid for rowid tables, primary-key columns for WITHOUT ROWID tables), which
// otherwise carry cid >= 0 and would inflate the reported column list.
// Expression parts (cid == -2, NULL name) are surfaced as "(expression)".
func (d *sqliteDriver) sqliteIndexColumns(ctx context.Context, indexName string) ([]string, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT cid, name FROM pragma_index_xinfo(?) WHERE key = 1 ORDER BY seqno`, indexName)
	if err != nil {
		return nil, fmt.Errorf("sqlite: object index columns: %w", err)
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var cid int
		var name sql.NullString
		if err := rows.Scan(&cid, &name); err != nil {
			return nil, fmt.Errorf("sqlite: object index columns scan: %w", err)
		}
		switch {
		case cid == -2:
			columns = append(columns, "(expression)")
		case name.Valid:
			columns = append(columns, name.String)
		}
	}
	return columns, rows.Err()
}

func sqliteSetColAttr(col *metadata.Column, key string, value any) {
	if col.Attributes == nil {
		col.Attributes = map[string]any{}
	}
	col.Attributes[key] = value
}

// sqliteTableEnrichable is the per-table detail recovered by re-parsing the
// stored CREATE TABLE text: constraint expressions and table options that the
// PRAGMA introspection does not expose.
type sqliteTableEnrichable struct {
	options   []string
	checks    []string
	collation map[string]string
	colChecks map[string]string
}

// sqliteTableEnrichment fetches the stored DDL for ref and re-parses it. Any
// failure (missing row, unparseable DDL, non-CREATE-TABLE statement) yields nil
// so introspection degrades to the PRAGMA-only view rather than erroring.
func (d *sqliteDriver) sqliteTableEnrichment(ctx context.Context, ref metadata.ObjectRef) *sqliteTableEnrichable {
	ns := ref.Scope.Name("database")
	// Identifiers cannot be bound; sqliteQuoteIdent escapes the namespace.
	// codeql[go/sql-injection]
	q := fmt.Sprintf(`SELECT sql FROM %s.sqlite_master WHERE type = 'table' AND name = ?`, sqliteQuoteIdent(ns))
	var ddl sql.NullString
	if err := d.db.QueryRowContext(ctx, q, ref.Name).Scan(&ddl); err != nil || !ddl.Valid || ddl.String == "" {
		return nil
	}
	stmts, _, err := parseSQLite(ctx, ddl.String)
	if err != nil || len(stmts) == 0 {
		return nil
	}
	create, ok := stmts[0].(*rqlitesql.CreateTableStatement)
	if !ok {
		return nil
	}

	e := &sqliteTableEnrichable{collation: map[string]string{}, colChecks: map[string]string{}}
	if create.Without.IsValid() && create.Rowid.IsValid() {
		e.options = append(e.options, "WITHOUT ROWID")
	}
	if create.Strict.IsValid() {
		e.options = append(e.options, "STRICT")
	}
	for _, col := range create.Columns {
		if col == nil || col.Name == nil {
			continue
		}
		for _, cons := range col.Constraints {
			switch c := cons.(type) {
			case *rqlitesql.CollateConstraint:
				if c.Collation != nil {
					e.collation[col.Name.Name] = c.Collation.Name
				}
			case *rqlitesql.CheckConstraint:
				if c.Expr != nil {
					expr := c.Expr.String()
					e.colChecks[col.Name.Name] = expr
					e.checks = append(e.checks, expr)
				}
			case *rqlitesql.PrimaryKeyConstraint:
				if c.Autoincrement.IsValid() {
					e.options = append(e.options, "AUTOINCREMENT")
				}
			}
		}
	}
	for _, cons := range create.Constraints {
		switch c := cons.(type) {
		case *rqlitesql.CheckConstraint:
			if c.Expr != nil {
				e.checks = append(e.checks, c.Expr.String())
			}
		case *rqlitesql.PrimaryKeyConstraint:
			if c.Autoincrement.IsValid() {
				e.options = append(e.options, "AUTOINCREMENT")
			}
		}
	}
	return e
}

// sqliteConstraintDescriptors folds the re-parsed enrichment and the PRAGMA-
// derived generated column names into a single "Constraints" fields descriptor.
// Returns nil when nothing was recovered.
func sqliteConstraintDescriptors(e *sqliteTableEnrichable, generatedCols []string) []metadata.Descriptor {
	var fields []metadata.Field
	if e != nil {
		if len(e.options) > 0 {
			fields = append(fields, metadata.Field{Name: "Options", Value: strings.Join(e.options, ", ")})
		}
		if len(e.checks) > 0 {
			fields = append(fields, metadata.Field{Name: "Checks", Value: strings.Join(e.checks, ", ")})
		}
	}
	if len(generatedCols) > 0 {
		fields = append(fields, metadata.Field{Name: "Generated", Value: strings.Join(generatedCols, ", ")})
	}
	if len(fields) == 0 {
		return nil
	}
	return []metadata.Descriptor{{Kind: "fields", Title: "Constraints", Fields: fields}}
}

func (d *sqliteDriver) inspectSQLiteTriggers(ctx context.Context, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	var out []metadata.Object
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
		obj := metadata.Object{
			Ref: ref,
			Descriptors: []metadata.Descriptor{
				{Kind: "fields", Title: "Trigger", Fields: []metadata.Field{{Name: "Table", Value: tableName}}},
			},
		}
		if definition.Valid && definition.String != "" {
			obj.Descriptors = append(obj.Descriptors, metadata.Descriptor{
				Kind:   "source",
				Title:  "Definition",
				Source: &metadata.Source{Language: "sql", Body: definition.String},
			})
		}
		out = append(out, obj)
	}
	return out, nil
}
