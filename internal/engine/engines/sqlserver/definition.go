package sqlserver

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/sqlwarden/internal/engine/metadata"
)

// sqlServerTableDDL reconstructs CREATE TABLE DDL for ref from the same
// catalog detail the object viewer shows: columns (types, NOT NULL,
// defaults, identity, computed), primary key, foreign keys, and secondary
// indexes. It does not cover CHECK constraints, partitioning, or storage
// options — output stays valid SQL, but is not a full scripting-fidelity
// reproduction. Returns ("", nil) if ref no longer exists.
func sqlServerTableDDL(ctx context.Context, db *sql.DB, ref metadata.ObjectRef) (string, error) {
	objects, err := RelationalObjects(ctx, db, []metadata.ObjectRef{ref})
	if err != nil {
		return "", err
	}
	if len(objects) == 0 || objects[0].Relational == nil {
		return "", nil
	}
	rel := objects[0].Relational

	var lines []string
	for _, col := range rel.Columns {
		if computed, ok := col.Attributes["computed"].(string); ok && computed != "" {
			lines = append(lines, "  "+sqlServerQuoteIdent(col.Name)+" AS "+computed)
			continue
		}
		line := "  " + sqlServerQuoteIdent(col.Name) + " " + col.DataType
		if identity, ok := col.Attributes["identity"].(string); ok && identity != "" {
			line += " IDENTITY(" + strings.ReplaceAll(identity, ",", ", ") + ")"
		}
		if !col.Nullable {
			line += " NOT NULL"
		}
		if col.Default != nil && *col.Default != "" {
			line += " DEFAULT " + *col.Default
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return "", nil
	}

	if len(rel.PrimaryKey) > 0 {
		quoted := make([]string, len(rel.PrimaryKey))
		for i, c := range rel.PrimaryKey {
			quoted[i] = sqlServerQuoteIdent(c)
		}
		lines = append(lines, "  PRIMARY KEY ("+strings.Join(quoted, ", ")+")")
	}
	for _, fk := range rel.ForeignKeys {
		cols := make([]string, len(fk.Columns))
		for i, c := range fk.Columns {
			cols[i] = sqlServerQuoteIdent(c)
		}
		refCols := make([]string, len(fk.ReferencedColumns))
		for i, c := range fk.ReferencedColumns {
			refCols[i] = sqlServerQuoteIdent(c)
		}
		lines = append(lines, fmt.Sprintf("  CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)",
			sqlServerQuoteIdent(fk.Name), strings.Join(cols, ", "),
			sqlServerQuoteQualified(fk.References.Scope.Name("schema"), fk.References.Name),
			strings.Join(refCols, ", ")))
	}

	var b strings.Builder
	b.WriteString("CREATE TABLE " + sqlServerQuoteQualified(ref.Scope.Name("schema"), ref.Name) + " (\n" +
		strings.Join(lines, ",\n") + "\n);")

	for _, idx := range rel.Indexes {
		cols := make([]string, len(idx.Columns))
		for i, c := range idx.Columns {
			cols[i] = sqlServerQuoteIdent(c)
		}
		unique := ""
		if idx.Unique {
			unique = "UNIQUE "
		}
		fmt.Fprintf(&b, "\n\nCREATE %sINDEX %s ON %s (%s);",
			unique, sqlServerQuoteIdent(idx.Name),
			sqlServerQuoteQualified(ref.Scope.Name("schema"), ref.Name), strings.Join(cols, ", "))
	}

	return b.String(), nil
}
