package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/sqlwarden/internal/engine/metadata"
	build "github.com/sqlwarden/internal/engine/metadata/build"
)

var (
	_ metadata.SchemaInspector = (*oracleDriver)(nil)
	_ metadata.ScopeDiscoverer = (*oracleDriver)(nil)
)

// oracleSystemSchemas are Oracle-maintained schema owners excluded from
// listings. The canonical runtime source is
// SELECT username FROM all_users WHERE oracle_maintained = 'Y' (12.2+);
// this static set is the fallback and the ALL_* query filter.
var oracleSystemSchemas = map[string]struct{}{
	"SYS": {}, "SYSTEM": {}, "XDB": {}, "CTXSYS": {}, "MDSYS": {}, "OUTLN": {},
	"DBSNMP": {}, "APPQOSSYS": {}, "GSMADMIN_INTERNAL": {}, "AUDSYS": {},
	"LBACSYS": {}, "DVSYS": {}, "ORDSYS": {}, "ORDDATA": {}, "WMSYS": {},
	"OJVMSYS": {}, "DBSFWUSER": {}, "REMOTE_SCHEDULER_AGENT": {}, "SYS$UMF": {},
	"ANONYMOUS": {}, "APEX_PUBLIC_USER": {}, "FLOWS_FILES": {}, "OLAPSYS": {},
	"SI_INFORMTN_SCHEMA": {}, "DIP": {}, "ORACLE_OCM": {}, "XS$NULL": {},
}

const oracleObjectKindTable = "table"

// oracleSystemSchemaList is the sorted form of oracleSystemSchemas, used to bind
// the owner exclusion set into dictionary queries.
func oracleSystemSchemaList() []string {
	out := make([]string, 0, len(oracleSystemSchemas))
	for name := range oracleSystemSchemas {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func (d *oracleDriver) SchemaSpec() metadata.SchemaSpec {
	return metadata.SchemaSpec{
		Dialect: "oracle",
		Kinds: []metadata.SchemaObjectKind{
			{Kind: "table", Label: "Table", PluralLabel: "Tables", Order: 1, Relational: true, SupportsDiagram: true, Listing: "enumerated"},
			{Kind: "view", Label: "View", PluralLabel: "Views", Order: 2, Relational: true, SupportsDiagram: true, Listing: "enumerated"},
			{Kind: "materialized_view", Label: "Materialized View", PluralLabel: "Materialized Views", Order: 3, Relational: true, SupportsDiagram: false, Listing: "enumerated"},
			{Kind: "sequence", Label: "Sequence", PluralLabel: "Sequences", Order: 4, Relational: false, SupportsDiagram: false, Listing: "enumerated"},
			{Kind: "function", Label: "Function", PluralLabel: "Functions", Order: 5, Relational: false, SupportsDiagram: false, Listing: "enumerated"},
			{Kind: "procedure", Label: "Procedure", PluralLabel: "Procedures", Order: 6, Relational: false, SupportsDiagram: false, Listing: "enumerated"},
			{Kind: "package", Label: "Package", PluralLabel: "Packages", Order: 7, Relational: false, SupportsDiagram: false, Listing: "enumerated"},
		},
	}
}

func (d *oracleDriver) currentSchema(ctx context.Context) (string, error) {
	var schema sql.NullString
	if err := d.db.QueryRowContext(ctx, `SELECT SYS_CONTEXT('USERENV','CURRENT_SCHEMA') FROM DUAL`).Scan(&schema); err != nil {
		return "", fmt.Errorf("oracle: current schema: %w", err)
	}
	return schema.String, nil
}

func (d *oracleDriver) InspectDirectory(ctx context.Context, opts metadata.DirectoryOptions) (*metadata.Directory, error) {
	owner := opts.Root.Name("schema")
	if owner == "" {
		owner = d.defaultScope.Name("schema")
	}
	if owner == "" {
		current, err := d.currentSchema(ctx)
		if err != nil {
			return nil, err
		}
		owner = current
	}
	if owner == "" {
		return &metadata.Directory{Engine: "oracle"}, nil
	}

	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: owner})
	b := build.NewDirectory()
	for _, kind := range []string{"table", "view", "materialized_view", "sequence", "function", "procedure", "package"} {
		b.DeclareKind(kind)
	}
	b.AddScope(scope)

	type listing struct {
		kind string
		sql  string
	}
	for _, l := range []listing{
		{"table", `SELECT table_name FROM all_tables WHERE owner = :1 ORDER BY table_name`},
		{"view", `SELECT view_name FROM all_views WHERE owner = :1 ORDER BY view_name`},
		{"materialized_view", `SELECT mview_name FROM all_mviews WHERE owner = :1 ORDER BY mview_name`},
		{"sequence", `SELECT sequence_name FROM all_sequences WHERE sequence_owner = :1 ORDER BY sequence_name`},
	} {
		if err := d.listInto(ctx, b, scope, l.kind, l.sql, owner); err != nil {
			return nil, err
		}
	}

	routineRows, err := d.db.QueryContext(ctx, `
SELECT object_name, object_type
FROM all_objects
WHERE owner = :1 AND object_type IN ('FUNCTION','PROCEDURE','PACKAGE') AND subobject_name IS NULL
ORDER BY object_type, object_name`, owner)
	if err != nil {
		return nil, fmt.Errorf("oracle: catalog routines: %w", err)
	}
	for routineRows.Next() {
		var name, objectType string
		if err := routineRows.Scan(&name, &objectType); err != nil {
			routineRows.Close()
			return nil, fmt.Errorf("oracle: catalog routines scan: %w", err)
		}
		b.AddRef(scope, strings.ToLower(objectType), name)
	}
	if err := routineRows.Err(); err != nil {
		routineRows.Close()
		return nil, fmt.Errorf("oracle: catalog routines rows: %w", err)
	}
	routineRows.Close()

	countRows, err := d.db.QueryContext(ctx, `
SELECT table_name, num_rows FROM all_tables WHERE owner = :1 AND num_rows IS NOT NULL`, owner)
	if err != nil {
		return nil, fmt.Errorf("oracle: catalog row counts: %w", err)
	}
	for countRows.Next() {
		var name string
		var count int64
		if err := countRows.Scan(&name, &count); err != nil {
			countRows.Close()
			return nil, fmt.Errorf("oracle: catalog row counts scan: %w", err)
		}
		b.SetRowCount(scope, oracleObjectKindTable, name, count)
	}
	if err := countRows.Err(); err != nil {
		countRows.Close()
		return nil, fmt.Errorf("oracle: catalog row counts rows: %w", err)
	}
	countRows.Close()

	return b.Build("", "oracle", scope), nil
}

func (d *oracleDriver) listInto(ctx context.Context, b *build.DirectoryBuilder, scope metadata.ScopePath, kind, query, owner string) error {
	rows, err := d.db.QueryContext(ctx, query, owner)
	if err != nil {
		return fmt.Errorf("oracle: catalog %s: %w", kind, err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("oracle: catalog %s scan: %w", kind, err)
		}
		b.AddRef(scope, kind, name)
	}
	return rows.Err()
}

func (d *oracleDriver) DiscoverScopes(ctx context.Context, request metadata.ScopeDiscoveryRequest) (*metadata.ScopeDiscovery, error) {
	current, err := d.currentSchema(ctx)
	if err != nil {
		return nil, err
	}
	result := &metadata.ScopeDiscovery{Scopes: []metadata.ScopePath{}}
	if current != "" {
		result.Current = metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: current})
	}
	if request.Parent != "" {
		return result, nil
	}
	systemOwners := oracleSystemSchemaList()
	placeholders := make([]string, len(systemOwners))
	args := make([]any, len(systemOwners))
	for i, owner := range systemOwners {
		placeholders[i] = ":" + strconv.Itoa(i+1)
		args[i] = owner
	}
	query := `SELECT DISTINCT owner FROM all_objects WHERE owner NOT IN (` +
		strings.Join(placeholders, ",") + `) ORDER BY owner`
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("oracle: discover schemas: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var owner string
		if err := rows.Scan(&owner); err != nil {
			return nil, fmt.Errorf("oracle: discover schemas scan: %w", err)
		}
		result.Scopes = append(result.Scopes, metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: owner}))
	}
	return result, rows.Err()
}

// oraclePairFilter builds an "(:s,:s+1),(:s+2,:s+3),..." predicate body for an
// (owner, object_name) IN (...) clause. args interleaves owner and name.
func oraclePairFilter(refs []metadata.ObjectRef, start int) (string, []any) {
	var sb strings.Builder
	args := make([]any, 0, len(refs)*2)
	n := start
	for i, ref := range refs {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString("(:" + strconv.Itoa(n) + ",:" + strconv.Itoa(n+1) + ")")
		n += 2
		args = append(args, ref.Scope.Name("schema"), ref.Name)
	}
	return sb.String(), args
}

func setObjectAttr(o *metadata.Object, key, value string) {
	if value == "" {
		return
	}
	if o.Attributes == nil {
		o.Attributes = map[string]any{}
	}
	o.Attributes[key] = value
}

func setColumnAttr(c *metadata.Column, key, value string) {
	if value == "" {
		return
	}
	if c.Attributes == nil {
		c.Attributes = map[string]any{}
	}
	c.Attributes[key] = value
}

func appendSource(o *metadata.Object, title, body string) {
	if body == "" {
		return
	}
	o.Descriptors = append(o.Descriptors, metadata.Descriptor{
		Kind:   "source",
		Title:  title,
		Source: &metadata.Source{Language: "sql", Body: body},
	})
}

func (d *oracleDriver) InspectObjects(ctx context.Context, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	var relational, mviews, sequences, routines []metadata.ObjectRef
	for _, ref := range refs {
		switch ref.Kind {
		case "table", "view":
			relational = append(relational, ref)
		case "materialized_view":
			mviews = append(mviews, ref)
		case "sequence":
			sequences = append(sequences, ref)
		case "function", "procedure", "package":
			routines = append(routines, ref)
		}
	}

	var out []metadata.Object
	if len(relational) > 0 {
		objs, err := d.inspectRelational(ctx, relational)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	if len(mviews) > 0 {
		objs, err := d.inspectMaterializedViews(ctx, mviews)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	if len(sequences) > 0 {
		objs, err := d.inspectSequences(ctx, sequences)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	if len(routines) > 0 {
		objs, err := d.inspectRoutines(ctx, routines)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	return out, nil
}

// oracleColumnType renders the data-dictionary type name with its precision or
// length parameters, matching how the type reads in DDL.
func oracleColumnType(dataType string, length, precision, scale sql.NullInt64) string {
	switch dataType {
	case "NUMBER":
		if precision.Valid {
			if scale.Valid && scale.Int64 != 0 {
				return fmt.Sprintf("NUMBER(%d,%d)", precision.Int64, scale.Int64)
			}
			return fmt.Sprintf("NUMBER(%d)", precision.Int64)
		}
		return "NUMBER"
	case "VARCHAR2", "NVARCHAR2", "CHAR", "NCHAR":
		if length.Valid {
			return fmt.Sprintf("%s(%d)", dataType, length.Int64)
		}
	}
	return dataType
}

func (d *oracleDriver) inspectRelational(ctx context.Context, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	refByName := make(map[string]metadata.ObjectRef, len(refs))
	for _, ref := range refs {
		refByName[ref.Scope.Name("schema")+"\x00"+ref.Name] = ref
	}
	refFor := func(owner, name string) metadata.ObjectRef {
		if ref, ok := refByName[owner+"\x00"+name]; ok {
			return ref
		}
		return metadata.ObjectRef{Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: owner}), Kind: oracleObjectKindTable, Name: name}
	}

	b := build.NewRelational()
	for _, ref := range refs {
		b.Ensure(ref)
	}

	pairs, args := oraclePairFilter(refs, 1)

	colQ := `
SELECT owner, table_name, column_name, data_type, data_length, data_precision,
       data_scale, nullable, data_default, column_id
FROM all_tab_columns
WHERE (owner, table_name) IN (` + pairs + `)
ORDER BY owner, table_name, column_id`
	crows, err := d.db.QueryContext(ctx, colQ, args...)
	if err != nil {
		return nil, fmt.Errorf("oracle: object columns: %w", err)
	}
	for crows.Next() {
		var owner, tbl, col, dataType, nullable string
		var length, precision, scale sql.NullInt64
		var def sql.NullString
		var columnID int
		if err := crows.Scan(&owner, &tbl, &col, &dataType, &length, &precision, &scale, &nullable, &def, &columnID); err != nil {
			crows.Close()
			return nil, fmt.Errorf("oracle: object columns scan: %w", err)
		}
		c := metadata.Column{
			Name:     col,
			DataType: oracleColumnType(dataType, length, precision, scale),
			Nullable: nullable == "Y",
			Ordinal:  columnID,
		}
		if def.Valid {
			v := strings.TrimRight(def.String, " \t\r\n")
			c.Default = &v
		}
		b.AddColumn(refFor(owner, tbl), c)
	}
	if err := crows.Err(); err != nil {
		crows.Close()
		return nil, fmt.Errorf("oracle: object columns rows: %w", err)
	}
	crows.Close()

	pkQ := `
SELECT cc.owner, cc.table_name, cc.column_name
FROM all_constraints c
JOIN all_cons_columns cc ON cc.owner = c.owner AND cc.constraint_name = c.constraint_name
WHERE c.constraint_type = 'P' AND (c.owner, c.table_name) IN (` + pairs + `)
ORDER BY cc.owner, cc.table_name, cc.position`
	prows, err := d.db.QueryContext(ctx, pkQ, args...)
	if err != nil {
		return nil, fmt.Errorf("oracle: object pk: %w", err)
	}
	for prows.Next() {
		var owner, tbl, col string
		if err := prows.Scan(&owner, &tbl, &col); err != nil {
			prows.Close()
			return nil, fmt.Errorf("oracle: object pk scan: %w", err)
		}
		b.AddPrimaryKeyColumn(refFor(owner, tbl), col)
	}
	if err := prows.Err(); err != nil {
		prows.Close()
		return nil, fmt.Errorf("oracle: object pk rows: %w", err)
	}
	prows.Close()

	fkQ := `
SELECT c.owner, c.table_name, c.constraint_name, cc.column_name,
       rc.owner AS ref_owner, rc.table_name AS ref_table, rcc.column_name AS ref_column
FROM all_constraints c
JOIN all_cons_columns cc ON cc.owner = c.owner AND cc.constraint_name = c.constraint_name
JOIN all_constraints rc ON rc.owner = c.r_owner AND rc.constraint_name = c.r_constraint_name
JOIN all_cons_columns rcc ON rcc.owner = rc.owner AND rcc.constraint_name = rc.constraint_name AND rcc.position = cc.position
WHERE c.constraint_type = 'R' AND (c.owner, c.table_name) IN (` + pairs + `)
ORDER BY c.owner, c.table_name, c.constraint_name, cc.position`
	frows, err := d.db.QueryContext(ctx, fkQ, args...)
	if err != nil {
		return nil, fmt.Errorf("oracle: object fk: %w", err)
	}
	for frows.Next() {
		var owner, tbl, name, col, refOwner, refTable, refColumn string
		if err := frows.Scan(&owner, &tbl, &name, &col, &refOwner, &refTable, &refColumn); err != nil {
			frows.Close()
			return nil, fmt.Errorf("oracle: object fk scan: %w", err)
		}
		b.AddForeignKeyColumn(refFor(owner, tbl), name, col,
			metadata.ObjectRef{Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: refOwner}), Kind: oracleObjectKindTable, Name: refTable}, refColumn)
	}
	if err := frows.Err(); err != nil {
		frows.Close()
		return nil, fmt.Errorf("oracle: object fk rows: %w", err)
	}
	frows.Close()

	idxQ := `
SELECT i.owner, i.table_name, i.index_name, i.uniqueness, ic.column_name, ic.column_position
FROM all_indexes i
JOIN all_ind_columns ic ON ic.index_owner = i.owner AND ic.index_name = i.index_name
WHERE (i.table_owner, i.table_name) IN (` + pairs + `)
  AND i.index_name NOT IN (
    SELECT index_name FROM all_constraints
    WHERE constraint_type = 'P' AND owner = i.table_owner
      AND table_name = i.table_name AND index_name IS NOT NULL
  )
ORDER BY i.owner, i.table_name, i.index_name, ic.column_position`
	irows, err := d.db.QueryContext(ctx, idxQ, args...)
	if err != nil {
		return nil, fmt.Errorf("oracle: object indexes: %w", err)
	}
	type idxKey struct{ owner, tbl, name string }
	indexes := map[idxKey]*metadata.SecondaryIndex{}
	var indexOrder []idxKey
	for irows.Next() {
		var owner, tbl, name, uniqueness, col string
		var position int
		if err := irows.Scan(&owner, &tbl, &name, &uniqueness, &col, &position); err != nil {
			irows.Close()
			return nil, fmt.Errorf("oracle: object index scan: %w", err)
		}
		key := idxKey{owner: owner, tbl: tbl, name: name}
		ix, ok := indexes[key]
		if !ok {
			ix = &metadata.SecondaryIndex{Name: name, Unique: uniqueness == "UNIQUE"}
			indexes[key] = ix
			indexOrder = append(indexOrder, key)
		}
		ix.Columns = append(ix.Columns, col)
	}
	if err := irows.Err(); err != nil {
		irows.Close()
		return nil, fmt.Errorf("oracle: object index rows: %w", err)
	}
	irows.Close()
	for _, key := range indexOrder {
		b.AddIndex(refFor(key.owner, key.tbl), *indexes[key])
	}

	out := b.Build()
	if err := d.attachOracleComments(ctx, out, pairs, args); err != nil {
		return nil, err
	}
	d.attachOracleRelationalDDL(ctx, out)
	return out, nil
}

// attachOracleComments populates table and column "comment" attributes from
// all_tab_comments / all_col_comments.
func (d *oracleDriver) attachOracleComments(ctx context.Context, objs []metadata.Object, pairs string, args []any) error {
	tableComments := map[string]string{}
	trows, err := d.db.QueryContext(ctx, `
SELECT owner, table_name, comments
FROM all_tab_comments
WHERE (owner, table_name) IN (`+pairs+`)`, args...)
	if err != nil {
		return fmt.Errorf("oracle: table comments: %w", err)
	}
	for trows.Next() {
		var owner, name string
		var comment sql.NullString
		if err := trows.Scan(&owner, &name, &comment); err != nil {
			trows.Close()
			return fmt.Errorf("oracle: table comments scan: %w", err)
		}
		if comment.Valid && comment.String != "" {
			tableComments[owner+"\x00"+name] = comment.String
		}
	}
	if err := trows.Err(); err != nil {
		trows.Close()
		return fmt.Errorf("oracle: table comments rows: %w", err)
	}
	trows.Close()

	type colKey struct{ owner, tbl, col string }
	colComments := map[colKey]string{}
	crows, err := d.db.QueryContext(ctx, `
SELECT owner, table_name, column_name, comments
FROM all_col_comments
WHERE (owner, table_name) IN (`+pairs+`)`, args...)
	if err != nil {
		return fmt.Errorf("oracle: column comments: %w", err)
	}
	for crows.Next() {
		var owner, tbl, col string
		var comment sql.NullString
		if err := crows.Scan(&owner, &tbl, &col, &comment); err != nil {
			crows.Close()
			return fmt.Errorf("oracle: column comments scan: %w", err)
		}
		if comment.Valid && comment.String != "" {
			colComments[colKey{owner: owner, tbl: tbl, col: col}] = comment.String
		}
	}
	if err := crows.Err(); err != nil {
		crows.Close()
		return fmt.Errorf("oracle: column comments rows: %w", err)
	}
	crows.Close()

	for i := range objs {
		owner, name := objs[i].Ref.Scope.Name("schema"), objs[i].Ref.Name
		if c := tableComments[owner+"\x00"+name]; c != "" {
			setObjectAttr(&objs[i], "comment", c)
		}
		if objs[i].Relational == nil {
			continue
		}
		for j := range objs[i].Relational.Columns {
			col := &objs[i].Relational.Columns[j]
			if c := colComments[colKey{owner: owner, tbl: name, col: col.Name}]; c != "" {
				setColumnAttr(col, "comment", c)
			}
		}
	}
	return nil
}

// attachOracleRelationalDDL appends a "DDL" source descriptor per table/view via
// DBMS_METADATA.GET_DDL. Retrieval is best-effort: an object whose DDL cannot be
// produced (insufficient privilege, unsupported storage) is left without the
// descriptor rather than failing the whole inspection.
func (d *oracleDriver) attachOracleRelationalDDL(ctx context.Context, objs []metadata.Object) {
	for i := range objs {
		owner := objs[i].Ref.Scope.Name("schema")
		name := objs[i].Ref.Name
		metadataType := "TABLE"
		if objs[i].Ref.Kind == "view" {
			metadataType = "VIEW"
		}
		var ddl sql.NullString
		err := d.db.QueryRowContext(ctx,
			`SELECT DBMS_METADATA.GET_DDL(:1, :2, :3) FROM DUAL`,
			metadataType, name, owner).Scan(&ddl)
		if err == nil && ddl.Valid && ddl.String != "" {
			appendSource(&objs[i], "DDL", ddl.String)
			continue
		}
		if objs[i].Ref.Kind != "view" {
			continue
		}
		var text sql.NullString
		if err := d.db.QueryRowContext(ctx,
			`SELECT text FROM all_views WHERE owner = :1 AND view_name = :2`,
			owner, name).Scan(&text); err != nil {
			continue
		}
		if text.Valid && text.String != "" {
			appendSource(&objs[i], "DDL", text.String)
		}
	}
}

func (d *oracleDriver) inspectMaterializedViews(ctx context.Context, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	pairs, args := oraclePairFilter(refs, 1)

	colQ := `
SELECT owner, table_name, column_name, data_type, data_length, data_precision,
       data_scale, nullable, data_default, column_id
FROM all_tab_columns
WHERE (owner, table_name) IN (` + pairs + `)
ORDER BY owner, table_name, column_id`
	rows, err := d.db.QueryContext(ctx, colQ, args...)
	if err != nil {
		return nil, fmt.Errorf("oracle: mview columns: %w", err)
	}
	columns := map[string][]metadata.Column{}
	for rows.Next() {
		var owner, mv, col, dataType, nullable string
		var length, precision, scale sql.NullInt64
		var def sql.NullString
		var columnID int
		if err := rows.Scan(&owner, &mv, &col, &dataType, &length, &precision, &scale, &nullable, &def, &columnID); err != nil {
			rows.Close()
			return nil, fmt.Errorf("oracle: mview columns scan: %w", err)
		}
		c := metadata.Column{
			Name:     col,
			DataType: oracleColumnType(dataType, length, precision, scale),
			Nullable: nullable == "Y",
			Ordinal:  columnID,
		}
		if def.Valid {
			v := strings.TrimRight(def.String, " \t\r\n")
			c.Default = &v
		}
		columns[owner+"\x00"+mv] = append(columns[owner+"\x00"+mv], c)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("oracle: mview columns rows: %w", err)
	}
	rows.Close()

	var out []metadata.Object
	for _, ref := range refs {
		owner := ref.Scope.Name("schema")
		obj := metadata.Object{Ref: ref}
		if cols := columns[owner+"\x00"+ref.Name]; len(cols) > 0 {
			obj.Relational = &metadata.RelationalDetail{Columns: cols}
		}
		var query sql.NullString
		if err := d.db.QueryRowContext(ctx,
			`SELECT query FROM all_mviews WHERE owner = :1 AND mview_name = :2`,
			owner, ref.Name).Scan(&query); err != nil {
			return nil, fmt.Errorf("oracle: mview query: %w", err)
		}
		if query.Valid && query.String != "" {
			obj.Descriptors = append(obj.Descriptors, metadata.Descriptor{
				Kind:   "source",
				Title:  "Definition",
				Source: &metadata.Source{Language: "sql", Body: query.String},
			})
		}
		out = append(out, obj)
	}
	return out, nil
}

func (d *oracleDriver) inspectSequences(ctx context.Context, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	var out []metadata.Object
	for _, ref := range refs {
		owner := ref.Scope.Name("schema")
		var minValue, maxValue, incrementBy, cacheSize, lastNumber sql.NullString
		err := d.db.QueryRowContext(ctx, `
SELECT min_value, max_value, increment_by, cache_size, last_number
FROM all_sequences
WHERE sequence_owner = :1 AND sequence_name = :2`,
			owner, ref.Name).Scan(&minValue, &maxValue, &incrementBy, &cacheSize, &lastNumber)
		if err != nil {
			return nil, fmt.Errorf("oracle: sequence detail: %w", err)
		}
		out = append(out, metadata.Object{
			Ref: ref,
			Descriptors: []metadata.Descriptor{
				{Kind: "fields", Title: "Sequence", Fields: []metadata.Field{
					{Name: "Min value", Value: minValue.String},
					{Name: "Max value", Value: maxValue.String},
					{Name: "Increment by", Value: incrementBy.String},
					{Name: "Cache size", Value: cacheSize.String},
					{Name: "Last number", Value: lastNumber.String},
				}},
			},
		})
	}
	return out, nil
}

func (d *oracleDriver) inspectRoutines(ctx context.Context, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	var out []metadata.Object
	for _, ref := range refs {
		owner := ref.Scope.Name("schema")
		objectType := strings.ToUpper(ref.Kind)

		srows, err := d.db.QueryContext(ctx, `
SELECT text FROM all_source
WHERE owner = :1 AND name = :2 AND type = :3
ORDER BY line`, owner, ref.Name, objectType)
		if err != nil {
			return nil, fmt.Errorf("oracle: routine source: %w", err)
		}
		var body strings.Builder
		for srows.Next() {
			var line sql.NullString
			if err := srows.Scan(&line); err != nil {
				srows.Close()
				return nil, fmt.Errorf("oracle: routine source scan: %w", err)
			}
			body.WriteString(line.String)
		}
		if err := srows.Err(); err != nil {
			srows.Close()
			return nil, fmt.Errorf("oracle: routine source rows: %w", err)
		}
		srows.Close()

		var status sql.NullString
		_ = d.db.QueryRowContext(ctx, `
SELECT status FROM all_objects
WHERE owner = :1 AND object_name = :2 AND object_type = :3`,
			owner, ref.Name, objectType).Scan(&status)

		obj := metadata.Object{
			Ref: ref,
			Descriptors: []metadata.Descriptor{
				{Kind: "fields", Title: "Routine", Fields: []metadata.Field{
					{Name: "Type", Value: objectType},
					{Name: "Status", Value: status.String},
				}},
			},
		}
		if src := body.String(); src != "" {
			obj.Descriptors = append(obj.Descriptors, metadata.Descriptor{
				Kind:   "source",
				Title:  "Source",
				Source: &metadata.Source{Language: "plsql", Body: src},
			})
		}
		out = append(out, obj)
	}
	return out, nil
}
