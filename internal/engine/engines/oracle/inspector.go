package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/sqlwarden/internal/engine/metadata"
	build "github.com/sqlwarden/internal/engine/metadata/build"
)

var (
	_ metadata.SchemaInspector     = (*oracleDriver)(nil)
	_ metadata.ScopeDiscoverer     = (*oracleDriver)(nil)
	_ metadata.DefinitionInspector = (*oracleDriver)(nil)
)

const oracleObjectKindTable = "table"

func (d *oracleDriver) SchemaSpec() metadata.SchemaSpec {
	return metadata.SchemaSpec{
		Dialect:       "oracle",
		BrowseScopes:  true,
		SystemSchemas: true,
		Kinds: []metadata.SchemaObjectKind{
			{Kind: "table", Label: "Table", PluralLabel: "Tables", Order: 1, Relational: true, SupportsDiagram: true, Listing: "enumerated", HasDefinition: true},
			{Kind: "view", Label: "View", PluralLabel: "Views", Order: 2, Relational: true, SupportsDiagram: true, Listing: "enumerated", HasDefinition: true},
			{Kind: "materialized_view", Label: "Materialized View", PluralLabel: "Materialized Views", Order: 3, Relational: true, SupportsDiagram: true, Listing: "enumerated", HasDefinition: true},
			{Kind: "sequence", Label: "Sequence", PluralLabel: "Sequences", Order: 4, Relational: false, SupportsDiagram: false, Listing: "enumerated", HasDefinition: true},
			{Kind: "function", Label: "Function", PluralLabel: "Functions", Order: 5, Relational: false, SupportsDiagram: false, Listing: "enumerated", HasDefinition: true},
			{Kind: "procedure", Label: "Procedure", PluralLabel: "Procedures", Order: 6, Relational: false, SupportsDiagram: false, Listing: "enumerated", HasDefinition: true},
			{Kind: "package", Label: "Package", PluralLabel: "Packages", Order: 7, Relational: false, SupportsDiagram: false, Listing: "enumerated", HasDefinition: true},
			{Kind: "package_body", Label: "Package Body", PluralLabel: "Package Bodies", Order: 8, Listing: "enumerated", HasDefinition: true},
			{Kind: "trigger", Label: "Trigger", PluralLabel: "Triggers", Order: 9, Listing: "enumerated", HasDefinition: true},
			{Kind: "type", Label: "Type", PluralLabel: "Types", Order: 10, Listing: "enumerated", HasDefinition: true},
			{Kind: "type_body", Label: "Type Body", PluralLabel: "Type Bodies", Order: 11, Listing: "enumerated", HasDefinition: true},
			{Kind: "synonym", Label: "Synonym", PluralLabel: "Synonyms", Order: 12, Listing: "enumerated", HasDefinition: true},
			{Kind: "db_link", Label: "Database Link", PluralLabel: "Database Links", Order: 13, Listing: "enumerated", HasDefinition: true},
			{Kind: "index", Label: "Index", PluralLabel: "Indexes", Order: 14, Listing: "enumerated", HasDefinition: true},
			{Kind: "constraint", Label: "Constraint", PluralLabel: "Constraints", Order: 15, Listing: "enumerated", HasDefinition: true},
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
	if opts.Root != "" {
		segments, err := opts.Root.Segments()
		if err != nil || len(segments) != 1 || segments[0].Kind != "schema" {
			return nil, fmt.Errorf("oracle: expected a schema scope")
		}
	}
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
	for _, kind := range d.SchemaSpec().Kinds {
		b.DeclareKind(kind.Kind)
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
		{"synonym", `SELECT synonym_name FROM all_synonyms WHERE owner = :1 ORDER BY synonym_name`},
		{"db_link", `SELECT db_link FROM all_db_links WHERE owner = :1 ORDER BY db_link`},
		{"index", `SELECT index_name FROM all_indexes WHERE owner = :1 ORDER BY index_name`},
		{"constraint", `SELECT constraint_name FROM all_constraints WHERE owner = :1 ORDER BY constraint_name`},
	} {
		if err := d.listInto(ctx, b, scope, l.kind, l.sql, owner); err != nil {
			return nil, err
		}
	}

	routineRows, err := d.db.QueryContext(ctx, `
SELECT object_name, object_type
FROM all_objects
WHERE owner = :1 AND object_type IN ('FUNCTION','PROCEDURE','PACKAGE','PACKAGE BODY','TRIGGER','TYPE','TYPE BODY') AND subobject_name IS NULL
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
		b.AddRef(scope, strings.ReplaceAll(strings.ToLower(objectType), " ", "_"), name)
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

	directory := b.Build("", "oracle", scope)
	if opts.Root == "" {
		nodes, err := d.visibleSchemas(ctx)
		if err != nil {
			return nil, err
		}
		for i := range nodes {
			if nodes[i].Path == scope {
				nodes[i].Groups = directory.Roots[0].Groups
				nodes[i].Lazy = false
			}
		}
		directory.Roots = nodes
	}
	return directory, nil
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

// oracleDict chooses between the privilege-aware ALL_* data-dictionary views and
// the cheaper owner-implicit USER_* views. USER_* views omit the owner column and
// skip the cross-schema privilege union that makes ALL_* expensive on accounts
// that can see many schemas; they are only valid when every object under
// inspection belongs to the connected schema.
type oracleDict struct{ user bool }

// objectDict reports whether refs can be served from USER_* views: they must all
// share one owner, and that owner must be the connected schema.
func (d *oracleDriver) objectDict(ctx context.Context, refs []metadata.ObjectRef) oracleDict {
	if len(refs) == 0 {
		return oracleDict{}
	}
	owner := refs[0].Scope.Name("schema")
	for _, ref := range refs[1:] {
		if ref.Scope.Name("schema") != owner {
			return oracleDict{}
		}
	}
	var login string
	err := d.db.QueryRowContext(ctx, `SELECT USER FROM DUAL`).Scan(&login)
	if err != nil || login == "" || login != owner {
		return oracleDict{}
	}
	return oracleDict{user: true}
}

// view maps an ALL_* dictionary view name to the active tier ("all_tab_columns"
// -> "user_tab_columns" in user-scoped mode).
func (dict oracleDict) view(allView string) string {
	if dict.user {
		return "user_" + strings.TrimPrefix(allView, "all_")
	}
	return allView
}

// ownerCol yields the owner expression for a projected row. USER_* views have no
// owner column, so the USER pseudo-column (the connected schema, which owns every
// row by construction) stands in for the ALL_* view's qualified owner column.
func (dict oracleDict) ownerCol(allCol string) string {
	if dict.user {
		return "USER"
	}
	return allCol
}

// ownerJoin is the "left = right AND " fragment tying two ALL_* views on owner;
// USER_* views are already single-schema and need no such predicate.
func (dict oracleDict) ownerJoin(left, right string) string {
	if dict.user {
		return ""
	}
	return left + " = " + right + " AND "
}

// objFilter builds the predicate restricting a dictionary view to refs, bound
// from :start. ALL_* matches (ownerCol, nameCol) pairs; USER_* matches nameCol
// alone.
func (dict oracleDict) objFilter(ownerCol, nameCol string, refs []metadata.ObjectRef, start int) (string, []any) {
	if dict.user {
		var sb strings.Builder
		sb.WriteString(nameCol + " IN (")
		args := make([]any, 0, len(refs))
		for i, ref := range refs {
			if i > 0 {
				sb.WriteString(",")
			}
			sb.WriteString(":" + strconv.Itoa(start+i))
			args = append(args, ref.Name)
		}
		sb.WriteString(")")
		return sb.String(), args
	}
	pairs, args := oraclePairFilter(refs, start)
	return "(" + ownerCol + ", " + nameCol + ") IN (" + pairs + ")", args
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

func oracleSourceDescriptor(title, body string) *metadata.Descriptor {
	if body == "" {
		return nil
	}
	return &metadata.Descriptor{
		Kind:   "source",
		Title:  title,
		Source: &metadata.Source{Language: "sql", Body: body},
	}
}

func (d *oracleDriver) InspectObjects(ctx context.Context, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	var relational, mviews, sequences, routines, catalog []metadata.ObjectRef
	for _, ref := range refs {
		switch ref.Kind {
		case "table", "view":
			relational = append(relational, ref)
		case "materialized_view":
			mviews = append(mviews, ref)
		case "sequence":
			sequences = append(sequences, ref)
		case "function", "procedure", "package", "package_body", "trigger", "type", "type_body":
			routines = append(routines, ref)
		case "synonym", "db_link", "index", "constraint":
			catalog = append(catalog, ref)
		}
	}

	dict := d.objectDict(ctx, refs)

	var out []metadata.Object
	if len(relational) > 0 {
		objs, err := d.inspectRelational(ctx, dict, relational)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	if len(mviews) > 0 {
		objs, err := d.inspectMaterializedViews(ctx, dict, mviews)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	if len(sequences) > 0 {
		objs, err := d.inspectSequences(ctx, dict, sequences)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	if len(routines) > 0 {
		objs, err := d.inspectRoutines(ctx, dict, routines)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	if len(catalog) > 0 {
		objs, err := d.inspectCatalogObjects(ctx, catalog)
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

func (d *oracleDriver) inspectRelational(ctx context.Context, dict oracleDict, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	refByName := make(map[string]metadata.ObjectRef, len(refs))
	for _, ref := range refs {
		refByName[ref.Scope.Name("schema")+"\x00"+ref.Name] = ref
	}
	// USER_* rows carry the USER pseudo-column instead of a real owner value;
	// pin every scanned owner to the single schema objectDict already verified so
	// refFor's map keys stay consistent with the request refs.
	userOwner := ""
	if dict.user && len(refs) > 0 {
		userOwner = refs[0].Scope.Name("schema")
	}
	ownerFor := func(scanned string) string {
		if dict.user {
			return userOwner
		}
		return scanned
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

	colFilter, colArgs := dict.objFilter("owner", "table_name", refs, 1)
	colQ := `
SELECT ` + dict.ownerCol("owner") + ` AS owner, table_name, column_name, data_type, data_length, data_precision,
       data_scale, nullable, data_default, column_id
FROM ` + dict.view("all_tab_columns") + `
WHERE ` + colFilter + `
ORDER BY table_name, column_id`
	crows, err := d.db.QueryContext(ctx, colQ, colArgs...)
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
		b.AddColumn(refFor(ownerFor(owner), tbl), c)
	}
	if err := crows.Err(); err != nil {
		crows.Close()
		return nil, fmt.Errorf("oracle: object columns rows: %w", err)
	}
	crows.Close()

	pkFilter, pkArgs := dict.objFilter("c.owner", "c.table_name", refs, 1)
	pkQ := `
SELECT ` + dict.ownerCol("cc.owner") + ` AS owner, cc.table_name, cc.column_name
FROM ` + dict.view("all_constraints") + ` c
JOIN ` + dict.view("all_cons_columns") + ` cc ON ` + dict.ownerJoin("cc.owner", "c.owner") + `cc.constraint_name = c.constraint_name
WHERE c.constraint_type = 'P' AND ` + pkFilter + `
ORDER BY cc.table_name, cc.position`
	prows, err := d.db.QueryContext(ctx, pkQ, pkArgs...)
	if err != nil {
		return nil, fmt.Errorf("oracle: object pk: %w", err)
	}
	for prows.Next() {
		var owner, tbl, col string
		if err := prows.Scan(&owner, &tbl, &col); err != nil {
			prows.Close()
			return nil, fmt.Errorf("oracle: object pk scan: %w", err)
		}
		b.AddPrimaryKeyColumn(refFor(ownerFor(owner), tbl), col)
	}
	if err := prows.Err(); err != nil {
		prows.Close()
		return nil, fmt.Errorf("oracle: object pk rows: %w", err)
	}
	prows.Close()

	fkFilter, fkArgs := dict.objFilter("c.owner", "c.table_name", refs, 1)
	// c/cc are restricted to the connected schema in USER_* mode, but the
	// referenced constraint (rc/rcc) can live in any schema, so it stays on the
	// privilege-aware ALL_* views with explicit owner predicates.
	fkQ := `
SELECT ` + dict.ownerCol("c.owner") + ` AS owner, c.table_name, c.constraint_name, cc.column_name,
       rc.owner AS ref_owner, rc.table_name AS ref_table, rcc.column_name AS ref_column
FROM ` + dict.view("all_constraints") + ` c
JOIN ` + dict.view("all_cons_columns") + ` cc ON ` + dict.ownerJoin("cc.owner", "c.owner") + `cc.constraint_name = c.constraint_name
JOIN all_constraints rc ON rc.owner = c.r_owner AND rc.constraint_name = c.r_constraint_name
JOIN all_cons_columns rcc ON rcc.owner = rc.owner AND rcc.constraint_name = rc.constraint_name AND rcc.position = cc.position
WHERE c.constraint_type = 'R' AND ` + fkFilter + `
ORDER BY c.table_name, c.constraint_name, cc.position`
	frows, err := d.db.QueryContext(ctx, fkQ, fkArgs...)
	if err != nil {
		return nil, fmt.Errorf("oracle: object fk: %w", err)
	}
	for frows.Next() {
		var owner, tbl, name, col, refOwner, refTable, refColumn string
		if err := frows.Scan(&owner, &tbl, &name, &col, &refOwner, &refTable, &refColumn); err != nil {
			frows.Close()
			return nil, fmt.Errorf("oracle: object fk scan: %w", err)
		}
		b.AddForeignKeyColumn(refFor(ownerFor(owner), tbl), name, col,
			metadata.ObjectRef{Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: refOwner}), Kind: oracleObjectKindTable, Name: refTable}, refColumn)
	}
	if err := frows.Err(); err != nil {
		frows.Close()
		return nil, fmt.Errorf("oracle: object fk rows: %w", err)
	}
	frows.Close()

	// The LEFT JOIN / IS NULL anti-join excludes primary-key-backing indexes
	// without the per-row correlated subquery the previous form used.
	idxFilter, idxArgs := dict.objFilter("i.table_owner", "i.table_name", refs, 1)
	idxQ := `
SELECT ` + dict.ownerCol("i.owner") + ` AS owner, i.table_name, i.index_name, i.uniqueness, ic.column_name, ic.column_position
FROM ` + dict.view("all_indexes") + ` i
JOIN ` + dict.view("all_ind_columns") + ` ic ON ` + dict.ownerJoin("ic.index_owner", "i.owner") + `ic.index_name = i.index_name
LEFT JOIN ` + dict.view("all_constraints") + ` pc
  ON ` + dict.ownerJoin("pc.owner", "i.table_owner") + `pc.table_name = i.table_name
 AND pc.index_name = i.index_name AND pc.constraint_type = 'P'
WHERE ` + idxFilter + `
  AND pc.index_name IS NULL
ORDER BY i.table_name, i.index_name, ic.column_position`
	irows, err := d.db.QueryContext(ctx, idxQ, idxArgs...)
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
		key := idxKey{owner: ownerFor(owner), tbl: tbl, name: name}
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
	if err := d.attachOracleComments(ctx, out, dict, refs); err != nil {
		return nil, err
	}
	if err := d.attachOracleTableDetails(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

// attachOracleComments populates table and column "comment" attributes from
// all_tab_comments / all_col_comments.
func (d *oracleDriver) attachOracleComments(ctx context.Context, objs []metadata.Object, dict oracleDict, refs []metadata.ObjectRef) error {
	userOwner := ""
	if dict.user && len(refs) > 0 {
		userOwner = refs[0].Scope.Name("schema")
	}
	ownerFor := func(scanned string) string {
		if dict.user {
			return userOwner
		}
		return scanned
	}
	tabFilter, tabArgs := dict.objFilter("owner", "table_name", refs, 1)
	tableComments := map[string]string{}
	trows, err := d.db.QueryContext(ctx, `
SELECT `+dict.ownerCol("owner")+` AS owner, table_name, comments
FROM `+dict.view("all_tab_comments")+`
WHERE `+tabFilter, tabArgs...)
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
			tableComments[ownerFor(owner)+"\x00"+name] = comment.String
		}
	}
	if err := trows.Err(); err != nil {
		trows.Close()
		return fmt.Errorf("oracle: table comments rows: %w", err)
	}
	trows.Close()

	type colKey struct{ owner, tbl, col string }
	colComments := map[colKey]string{}
	colFilter, colArgs := dict.objFilter("owner", "table_name", refs, 1)
	crows, err := d.db.QueryContext(ctx, `
SELECT `+dict.ownerCol("owner")+` AS owner, table_name, column_name, comments
FROM `+dict.view("all_col_comments")+`
WHERE `+colFilter, colArgs...)
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
			colComments[colKey{owner: ownerFor(owner), tbl: tbl, col: col}] = comment.String
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

// InspectDefinition fetches an object's DDL on demand via DBMS_METADATA.GET_DDL,
// so the bulk InspectObjects path (and every schema snapshot) avoids one round
// trip per object for text the UI needs only when a user opens an object's
// detail view. PL/SQL kinds (function, procedure, package, trigger, type)
// already carry their all_source text inline from InspectObjects and are not
// re-fetched here. Retrieval is best-effort: a kind without a retrievable
// definition, or a failure (insufficient privilege, unsupported storage),
// yields a nil descriptor rather than an error. Views fall back to
// all_views.text when GET_DDL is unavailable to the caller. Constraint DDL is
// reconstructed from all_constraints/all_cons_columns rather than GET_DDL.
func (d *oracleDriver) InspectDefinition(ctx context.Context, ref metadata.ObjectRef) (*metadata.Descriptor, error) {
	owner := ref.Scope.Name("schema")
	name := ref.Name

	var metadataType string
	switch ref.Kind {
	case "table":
		metadataType = "TABLE"
	case "view":
		metadataType = "VIEW"
	case "materialized_view":
		metadataType = "MATERIALIZED_VIEW"
	case "sequence":
		metadataType = "SEQUENCE"
	case "synonym":
		metadataType = "SYNONYM"
	case "db_link":
		metadataType = "DB_LINK"
	case "index":
		metadataType = "INDEX"
	case "constraint":
		return d.oracleConstraintDefinition(ctx, owner, name)
	default:
		return nil, nil
	}

	var ddl sql.NullString
	if err := d.db.QueryRowContext(ctx,
		`SELECT DBMS_METADATA.GET_DDL(:1, :2, :3) FROM DUAL`,
		metadataType, name, owner).Scan(&ddl); err == nil && ddl.Valid {
		if desc := oracleSourceDescriptor("DDL", ddl.String); desc != nil {
			return desc, nil
		}
	}

	if ref.Kind != "view" {
		return nil, nil
	}
	var text sql.NullString
	if err := d.db.QueryRowContext(ctx,
		`SELECT text FROM all_views WHERE owner = :1 AND view_name = :2`,
		owner, name).Scan(&text); err != nil {
		return nil, nil
	}
	return oracleSourceDescriptor("DDL", text.String), nil
}

// oracleConstraintDefinition reconstructs an ALTER TABLE ... ADD CONSTRAINT
// statement from all_constraints/all_cons_columns for a P/U/R/C constraint,
// or a nil descriptor if the name is not one of those (NOT NULL checks
// included) or no longer exists. NOT NULL checks (search_condition of the form
// "COL IS NOT NULL") are skipped since they belong in the column definition.
// Deferrable state, MATCH, and constraint status/validation are not reproduced.
func (d *oracleDriver) oracleConstraintDefinition(ctx context.Context, owner, name string) (*metadata.Descriptor, error) {
	var (
		conType   string
		tableName string
		search    sql.NullString
		rOwner    sql.NullString
		rConName  sql.NullString
		deleteRul sql.NullString
	)
	if err := d.db.QueryRowContext(ctx, `
SELECT constraint_type, table_name, search_condition_vc, r_owner, r_constraint_name, delete_rule
FROM all_constraints
WHERE owner = :1 AND constraint_name = :2`,
		owner, name).Scan(&conType, &tableName, &search, &rOwner, &rConName, &deleteRul); err != nil {
		return nil, nil
	}

	cols, err := d.oracleConsColumns(ctx, owner, name)
	if err != nil {
		return nil, nil
	}

	head := fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s ",
		oracleQualified(owner, tableName), oracleQuoteIdent(name))

	switch conType {
	case "P":
		return oracleSourceDescriptor("DDL", head+"PRIMARY KEY ("+strings.Join(cols, ", ")+");"), nil
	case "U":
		return oracleSourceDescriptor("DDL", head+"UNIQUE ("+strings.Join(cols, ", ")+");"), nil
	case "C":
		cond := strings.TrimSpace(search.String)
		if cond == "" || isOracleNotNullCondition(cond) {
			return nil, nil
		}
		return oracleSourceDescriptor("DDL", head+"CHECK ("+cond+");"), nil
	case "R":
		refCols, err := d.oracleConsColumns(ctx, rOwner.String, rConName.String)
		if err != nil {
			return nil, nil
		}
		var refTable string
		if err := d.db.QueryRowContext(ctx,
			`SELECT table_name FROM all_constraints WHERE owner = :1 AND constraint_name = :2`,
			rOwner.String, rConName.String).Scan(&refTable); err != nil {
			return nil, nil
		}
		stmt := head + "FOREIGN KEY (" + strings.Join(cols, ", ") + ") REFERENCES " +
			oracleQualified(rOwner.String, refTable) + " (" + strings.Join(refCols, ", ") + ")"
		if r := strings.ToUpper(deleteRul.String); r == "CASCADE" || r == "SET NULL" {
			stmt += " ON DELETE " + r
		}
		return oracleSourceDescriptor("DDL", stmt+";"), nil
	default:
		return nil, nil
	}
}

// oracleConsColumns returns the quoted column names of a constraint in key
// position order.
func (d *oracleDriver) oracleConsColumns(ctx context.Context, owner, name string) ([]string, error) {
	rows, err := d.db.QueryContext(ctx, `
SELECT column_name
FROM all_cons_columns
WHERE owner = :1 AND constraint_name = :2
ORDER BY position`, owner, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			return nil, err
		}
		cols = append(cols, oracleQuoteIdent(col))
	}
	return cols, rows.Err()
}

// isOracleNotNullCondition reports whether an all_constraints search condition
// is just a system-generated NOT NULL check ("COL" IS NOT NULL).
func isOracleNotNullCondition(cond string) bool {
	return strings.HasSuffix(strings.ToUpper(strings.TrimSpace(cond)), "IS NOT NULL")
}

func (d *oracleDriver) inspectMaterializedViews(ctx context.Context, dict oracleDict, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	userOwner := ""
	if dict.user && len(refs) > 0 {
		userOwner = refs[0].Scope.Name("schema")
	}
	ownerFor := func(scanned string) string {
		if dict.user {
			return userOwner
		}
		return scanned
	}

	colFilter, colArgs := dict.objFilter("owner", "table_name", refs, 1)
	colQ := `
SELECT ` + dict.ownerCol("owner") + ` AS owner, table_name, column_name, data_type, data_length, data_precision,
       data_scale, nullable, data_default, column_id
FROM ` + dict.view("all_tab_columns") + `
WHERE ` + colFilter + `
ORDER BY table_name, column_id`
	rows, err := d.db.QueryContext(ctx, colQ, colArgs...)
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
		key := ownerFor(owner) + "\x00" + mv
		columns[key] = append(columns[key], c)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("oracle: mview columns rows: %w", err)
	}
	rows.Close()

	// all_mviews.query is a LONG column; go-ora requires it to be the last
	// selected column, and only one such column per statement.
	defFilter, defArgs := dict.objFilter("owner", "mview_name", refs, 1)
	defQ := `
SELECT ` + dict.ownerCol("owner") + ` AS owner, mview_name, query
FROM ` + dict.view("all_mviews") + `
WHERE ` + defFilter
	drows, err := d.db.QueryContext(ctx, defQ, defArgs...)
	if err != nil {
		return nil, fmt.Errorf("oracle: mview query: %w", err)
	}
	definitions := map[string]string{}
	for drows.Next() {
		var owner, mv string
		var query sql.NullString
		if err := drows.Scan(&owner, &mv, &query); err != nil {
			drows.Close()
			return nil, fmt.Errorf("oracle: mview query scan: %w", err)
		}
		if query.Valid && query.String != "" {
			definitions[ownerFor(owner)+"\x00"+mv] = query.String
		}
	}
	if err := drows.Err(); err != nil {
		drows.Close()
		return nil, fmt.Errorf("oracle: mview query rows: %w", err)
	}
	drows.Close()

	var out []metadata.Object
	for _, ref := range refs {
		owner := ref.Scope.Name("schema")
		obj := metadata.Object{Ref: ref}
		if cols := columns[owner+"\x00"+ref.Name]; len(cols) > 0 {
			obj.Relational = &metadata.RelationalDetail{Columns: cols}
		}
		if body := definitions[owner+"\x00"+ref.Name]; body != "" {
			obj.Descriptors = append(obj.Descriptors, metadata.Descriptor{
				Kind:   "source",
				Title:  "Definition",
				Source: &metadata.Source{Language: "sql", Body: body},
			})
		}
		out = append(out, obj)
	}
	return out, nil
}

func (d *oracleDriver) inspectSequences(ctx context.Context, dict oracleDict, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	userOwner := ""
	if dict.user && len(refs) > 0 {
		userOwner = refs[0].Scope.Name("schema")
	}
	ownerFor := func(scanned string) string {
		if dict.user {
			return userOwner
		}
		return scanned
	}

	seqFilter, seqArgs := dict.objFilter("sequence_owner", "sequence_name", refs, 1)
	rows, err := d.db.QueryContext(ctx, `
SELECT `+dict.ownerCol("sequence_owner")+` AS sequence_owner, sequence_name, min_value, max_value, increment_by, cache_size, last_number
FROM `+dict.view("all_sequences")+`
WHERE `+seqFilter, seqArgs...)
	if err != nil {
		return nil, fmt.Errorf("oracle: sequence detail: %w", err)
	}
	defer rows.Close()

	type seqDetail struct{ minValue, maxValue, incrementBy, cacheSize, lastNumber string }
	detail := map[string]seqDetail{}
	for rows.Next() {
		var owner, name string
		var minValue, maxValue, incrementBy, cacheSize, lastNumber sql.NullString
		if err := rows.Scan(&owner, &name, &minValue, &maxValue, &incrementBy, &cacheSize, &lastNumber); err != nil {
			return nil, fmt.Errorf("oracle: sequence detail scan: %w", err)
		}
		detail[ownerFor(owner)+"\x00"+name] = seqDetail{minValue.String, maxValue.String, incrementBy.String, cacheSize.String, lastNumber.String}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("oracle: sequence detail rows: %w", err)
	}

	out := make([]metadata.Object, 0, len(refs))
	for _, ref := range refs {
		s := detail[ref.Scope.Name("schema")+"\x00"+ref.Name]
		out = append(out, metadata.Object{
			Ref: ref,
			Descriptors: []metadata.Descriptor{
				{Kind: "fields", Title: "Sequence", Fields: []metadata.Field{
					{Name: "Min value", Value: s.minValue},
					{Name: "Max value", Value: s.maxValue},
					{Name: "Increment by", Value: s.incrementBy},
					{Name: "Cache size", Value: s.cacheSize},
					{Name: "Last number", Value: s.lastNumber},
				}},
			},
		})
	}
	return out, nil
}

func (d *oracleDriver) inspectRoutines(ctx context.Context, dict oracleDict, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	userOwner := ""
	if dict.user && len(refs) > 0 {
		userOwner = refs[0].Scope.Name("schema")
	}
	ownerFor := func(scanned string) string {
		if dict.user {
			return userOwner
		}
		return scanned
	}

	srcFilter, srcArgs := dict.objFilter("owner", "name", refs, 1)
	// One pass over all_source for every requested routine. Keyed by
	// (owner, name, type) so a PACKAGE ref keeps only its spec source, matching
	// the previous per-routine type filter.
	srcRows, err := d.db.QueryContext(ctx, `
SELECT `+dict.ownerCol("owner")+` AS owner, name, type, text FROM `+dict.view("all_source")+`
WHERE `+srcFilter+`
ORDER BY name, type, line`, srcArgs...)
	if err != nil {
		return nil, fmt.Errorf("oracle: routine source: %w", err)
	}
	bodies := map[string]*strings.Builder{}
	for srcRows.Next() {
		var owner, name, typ string
		var line sql.NullString
		if err := srcRows.Scan(&owner, &name, &typ, &line); err != nil {
			srcRows.Close()
			return nil, fmt.Errorf("oracle: routine source scan: %w", err)
		}
		key := ownerFor(owner) + "\x00" + name + "\x00" + typ
		b := bodies[key]
		if b == nil {
			b = &strings.Builder{}
			bodies[key] = b
		}
		b.WriteString(line.String)
	}
	if err := srcRows.Err(); err != nil {
		srcRows.Close()
		return nil, fmt.Errorf("oracle: routine source rows: %w", err)
	}
	srcRows.Close()

	statusFilter, statusArgs := dict.objFilter("owner", "object_name", refs, 1)
	statusRows, err := d.db.QueryContext(ctx, `
SELECT `+dict.ownerCol("owner")+` AS owner, object_name, object_type, status FROM `+dict.view("all_objects")+`
WHERE `+statusFilter, statusArgs...)
	if err != nil {
		return nil, fmt.Errorf("oracle: routine status: %w", err)
	}
	statuses := map[string]string{}
	for statusRows.Next() {
		var owner, name, typ string
		var status sql.NullString
		if err := statusRows.Scan(&owner, &name, &typ, &status); err != nil {
			statusRows.Close()
			return nil, fmt.Errorf("oracle: routine status scan: %w", err)
		}
		statuses[ownerFor(owner)+"\x00"+name+"\x00"+typ] = status.String
	}
	if err := statusRows.Err(); err != nil {
		statusRows.Close()
		return nil, fmt.Errorf("oracle: routine status rows: %w", err)
	}
	statusRows.Close()

	out := make([]metadata.Object, 0, len(refs))
	for _, ref := range refs {
		objectType := strings.ReplaceAll(strings.ToUpper(ref.Kind), "_", " ")
		key := ref.Scope.Name("schema") + "\x00" + ref.Name + "\x00" + objectType
		obj := metadata.Object{
			Ref: ref,
			Descriptors: []metadata.Descriptor{
				{Kind: "fields", Title: "Routine", Fields: []metadata.Field{
					{Name: "Type", Value: objectType},
					{Name: "Status", Value: statuses[key]},
				}},
			},
		}
		if b := bodies[key]; b != nil {
			if src := b.String(); src != "" {
				obj.Descriptors = append(obj.Descriptors, metadata.Descriptor{
					Kind:   "source",
					Title:  "Source",
					Source: &metadata.Source{Language: "plsql", Body: src},
				})
			}
		}
		out = append(out, obj)
	}
	return out, nil
}
