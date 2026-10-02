// Package mssql adapts Bytebase/Omni's SQL Server completion to SQLWarden's
// dialect-neutral completion boundary. Omni's mssql completion resolves no
// catalog names, so keyword and type candidates come from
// mssqlcompletion.Complete and every object name is resolved from SQLWarden's
// metadata through completioncore.SchemaResolver, driven by the intent and
// scope parser.CollectCompletion reports.
package mssql

import (
	"context"
	"sort"
	"strings"

	mssqlast "github.com/bytebase/omni/mssql/ast"
	mssqlcompletion "github.com/bytebase/omni/mssql/completion"
	mssqlparser "github.com/bytebase/omni/mssql/parser"

	"github.com/sqlwarden/internal/engine/completioncore"
)

// ReturnsTableAttribute marks a function listing entry whose result is a
// table, so it completes as a row source rather than an expression.
const ReturnsTableAttribute = "returns_table"

type wants struct {
	relations, columns, functions, procedures, databases, schemas bool
}

func intentOf(intent *mssqlparser.CompletionIntent) wants {
	var w wants
	for _, kind := range intent.ObjectKinds {
		switch kind {
		case mssqlparser.ObjectKindTable, mssqlparser.ObjectKindView:
			w.relations = true
		case mssqlparser.ObjectKindColumn:
			w.columns = true
		case mssqlparser.ObjectKindFunction:
			w.functions = true
		case mssqlparser.ObjectKindProcedure:
			w.procedures = true
		case mssqlparser.ObjectKindDatabase:
			w.databases = true
		case mssqlparser.ObjectKindSchema:
			w.schemas = true
		}
	}
	return w
}

func Complete(
	ctx context.Context,
	sql string,
	cursor int,
	meta *completioncore.SchemaResolver,
) ([]completioncore.Candidate, completioncore.Context, error) {
	if err := completioncore.CheckContext(ctx); err != nil {
		return nil, completioncore.Context{}, err
	}
	cursor = max(0, min(cursor, len(sql)))
	start, _, prefix, suppressed := CursorWord(sql, cursor)
	if suppressed {
		return nil, completioncore.Context{Position: completioncore.PositionAny}, nil
	}

	var out []completioncore.Candidate
	for _, candidate := range mssqlcompletion.Complete(sql, cursor, nil) {
		switch candidate.Type {
		case mssqlcompletion.CandidateKeyword:
			out = append(out, completioncore.Candidate{Text: candidate.Text, Type: completioncore.CandidateKeyword})
		case mssqlcompletion.CandidateType_:
			out = append(out, completioncore.Candidate{Text: candidate.Text, Type: completioncore.CandidateTypeName})
		case mssqlcompletion.CandidateFunction:
			out = append(out, completioncore.Candidate{Text: candidate.Text, InsertText: candidate.Text, Type: completioncore.CandidateFunction, Definition: candidate.Definition})
		}
	}

	var w wants
	if meta != nil {
		if cc := mssqlparser.CollectCompletion(sql, start); cc != nil && cc.Intent != nil {
			c := &completion{sql: sql, start: start, cc: cc, meta: meta, want: intentOf(cc.Intent)}
			w = c.want
			out = append(out, c.objects()...)
		}
	}
	if err := completioncore.CheckContext(ctx); err != nil {
		return nil, completioncore.Context{}, err
	}
	out = filterByPrefix(dedupe(out), prefix)
	return out, completioncore.Context{Position: position(w, out)}, nil
}

type completion struct {
	sql   string
	start int
	cc    *mssqlparser.CompletionContext
	meta  *completioncore.SchemaResolver
	want  wants
}

// objects dispatches on the typed qualifier. Omni fills the qualifier from
// the right: one part lands in Schema, two in Database and Schema. The server
// part is ignored.
func (c *completion) objects() []completioncore.Candidate {
	q := c.cc.Intent.Qualifier
	switch {
	case q.Schema == "":
		return c.unqualified()
	case q.Database == "":
		return c.onePart(q.Schema)
	default:
		return c.twoPart(q.Database, q.Schema)
	}
}

func (c *completion) unqualified() []completioncore.Candidate {
	var out []completioncore.Candidate
	if c.want.relations {
		for _, cte := range c.cc.CTEs {
			out = append(out, completioncore.Candidate{Text: cte.Object, Type: completioncore.CandidateTable, Definition: "Common table expression"})
		}
		out = append(out, c.relations("", "")...)
		out = append(out, c.rowSources("", "")...)
		out = append(out, c.schemas("")...)
	}
	if c.want.columns {
		for _, ref := range c.visibleRefs() {
			out = append(out, c.refColumns(ref)...)
		}
	}
	if c.want.functions {
		out = append(out, c.scalarFunctions("", "", true)...)
	}
	if c.want.procedures {
		out = append(out, c.procedures("", "")...)
	}
	if c.want.databases {
		for _, name := range c.meta.DatabaseNames() {
			out = append(out, completioncore.Candidate{Text: name, Type: completioncore.CandidateDatabase})
		}
	}
	if c.want.schemas && !c.want.relations {
		out = append(out, c.schemas("")...)
	}
	return out
}

// onePart resolves "x.": an alias or bare relation in scope, then a schema of
// the default database, then a database whose schemas are offered.
func (c *completion) onePart(x string) []completioncore.Candidate {
	if c.want.columns {
		for _, ref := range c.visibleRefs() {
			if strings.EqualFold(ref.Alias, x) || (ref.Alias == "" && ref.Schema == "" && strings.EqualFold(ref.Object, x)) {
				return c.refColumns(ref)
			}
		}
	}
	out := c.inSchema("", x)
	if c.want.relations || c.want.procedures {
		for _, database := range c.meta.DatabaseNames() {
			if strings.EqualFold(database, x) {
				out = append(out, c.schemas(database)...)
				break
			}
		}
	}
	return out
}

func (c *completion) twoPart(database, schema string) []completioncore.Candidate {
	if c.want.columns {
		for _, ref := range c.visibleRefs() {
			if ref.Alias == "" && ref.Database == "" && strings.EqualFold(ref.Schema, database) && strings.EqualFold(ref.Object, schema) {
				return c.refColumns(ref)
			}
		}
	}
	return c.inSchema(database, schema)
}

func (c *completion) inSchema(database, schema string) []completioncore.Candidate {
	var out []completioncore.Candidate
	if c.want.relations {
		out = append(out, c.relations(database, schema)...)
		out = append(out, c.rowSources(database, schema)...)
	}
	if c.want.columns || c.want.functions {
		out = append(out, c.scalarFunctions(database, schema, false)...)
	}
	if c.want.procedures {
		out = append(out, c.procedures(database, schema)...)
	}
	return out
}

func (c *completion) relations(database, schema string) []completioncore.Candidate {
	var out []completioncore.Candidate
	for _, relation := range c.meta.Relations(database, schema) {
		out = append(out, completioncore.Candidate{
			Text: relation.Name, Type: relation.Kind,
			Definition: relation.Schema + " · " + string(relation.Kind),
		})
	}
	return out
}

// rowSources are the non-relation names valid after FROM: synonyms and
// table-valued functions.
func (c *completion) rowSources(database, schema string) []completioncore.Candidate {
	var out []completioncore.Candidate
	for _, ref := range c.meta.CatalogObjects(database, schema, "synonym", "function") {
		child, _ := c.meta.Child(ref)
		switch {
		case ref.Kind == "synonym":
			target, _ := child.Attributes["target"].(string)
			out = append(out, completioncore.Candidate{Text: ref.Name, Type: completioncore.CandidateSynonym, Definition: target})
		case child.Attributes[ReturnsTableAttribute] == true:
			out = append(out, completioncore.Candidate{Text: ref.Name, Type: completioncore.CandidateFunction, Definition: ref.Scope.Name("schema") + " · table-valued function"})
		}
	}
	return out
}

// scalarFunctions offers user functions that are not table-valued. SQL
// Server requires a schema on scalar user function calls, so unqualified
// candidates insert schema.name.
func (c *completion) scalarFunctions(database, schema string, qualify bool) []completioncore.Candidate {
	var out []completioncore.Candidate
	for _, ref := range c.meta.CatalogObjects(database, schema, "function") {
		if child, _ := c.meta.Child(ref); child.Attributes[ReturnsTableAttribute] == true {
			continue
		}
		owner := ref.Scope.Name("schema")
		candidate := completioncore.Candidate{Text: ref.Name, Type: completioncore.CandidateFunction, Definition: owner + " · function"}
		if qualify {
			candidate.InsertText = QuoteIdentifier(owner) + "." + QuoteIdentifier(ref.Name)
		}
		out = append(out, candidate)
	}
	return out
}

func (c *completion) procedures(database, schema string) []completioncore.Candidate {
	var out []completioncore.Candidate
	for _, ref := range c.meta.CatalogObjects(database, schema, "procedure") {
		out = append(out, completioncore.Candidate{Text: ref.Name, Type: completioncore.CandidateProcedure, Definition: ref.Scope.Name("schema") + " · procedure"})
	}
	return out
}

func (c *completion) schemas(database string) []completioncore.Candidate {
	var out []completioncore.Candidate
	for _, name := range c.meta.SchemaNames(database) {
		out = append(out, completioncore.Candidate{Text: name, Type: completioncore.CandidateSchema})
	}
	return out
}

// visibleRefs lists local then outer references, nearest first, without the
// reference being typed at the cursor.
func (c *completion) visibleRefs() []mssqlparser.RangeReference {
	if c.cc.Scope == nil {
		return nil
	}
	seen := map[mssqlast.Loc]bool{}
	var out []mssqlparser.RangeReference
	add := func(refs []mssqlparser.RangeReference) {
		for _, ref := range refs {
			if ref.Loc.Start <= c.start && c.start <= ref.Loc.End {
				continue
			}
			if seen[ref.Loc] {
				continue
			}
			seen[ref.Loc] = true
			out = append(out, ref)
		}
	}
	add(c.cc.Scope.LocalReferences)
	for _, outer := range c.cc.Scope.OuterReferences {
		add(outer)
	}
	return out
}

func (c *completion) refColumns(ref mssqlparser.RangeReference) []completioncore.Candidate {
	label := ref.Alias
	if label == "" {
		label = ref.Object
	}
	switch ref.Kind {
	case mssqlparser.RangeReferenceRelation, mssqlparser.RangeReferenceDMLTarget,
		mssqlparser.RangeReferenceMergeTarget, mssqlparser.RangeReferenceMergeSource:
		if ref.Database == "" && ref.Schema == "" {
			if cte, ok := c.cte(ref.Object); ok {
				return derivedCandidates(label, derivedColumns(c.sql, cte))
			}
		}
		if ref.Kind != mssqlparser.RangeReferenceRelation && c.isAlias(ref.Object) {
			return nil
		}
		if ref.Server != "" || strings.HasPrefix(ref.Object, "#") || strings.HasPrefix(ref.Object, "@") {
			return nil
		}
		relation, ok := c.meta.FindRelation(ref.Database, ref.Schema, ref.Object)
		if !ok {
			return nil
		}
		out := make([]completioncore.Candidate, 0, len(relation.Columns))
		for _, column := range relation.Columns {
			out = append(out, completioncore.Candidate{
				Text: column.Name, Type: completioncore.CandidateColumn,
				Definition: completioncore.ColumnDefinition(relation, column), Comment: column.Comment,
			})
		}
		return out
	case mssqlparser.RangeReferenceCTE:
		names := derivedColumns(c.sql, ref)
		if len(names) == 0 {
			if cte, ok := c.cte(ref.Object); ok {
				names = derivedColumns(c.sql, cte)
			}
		}
		return derivedCandidates(label, names)
	default:
		return derivedCandidates(label, derivedColumns(c.sql, ref))
	}
}

func (c *completion) cte(name string) (mssqlparser.RangeReference, bool) {
	for _, cte := range c.cc.CTEs {
		if strings.EqualFold(cte.Object, name) {
			return cte, true
		}
	}
	return mssqlparser.RangeReference{}, false
}

// isAlias reports whether name is another visible reference's alias, as in
// UPDATE o SET ... FROM orders o, where the DML target names the alias.
func (c *completion) isAlias(name string) bool {
	for _, ref := range c.visibleRefs() {
		if ref.Alias != "" && strings.EqualFold(ref.Alias, name) {
			return true
		}
	}
	return false
}

func derivedCandidates(owner string, names []string) []completioncore.Candidate {
	out := make([]completioncore.Candidate, 0, len(names))
	for _, name := range names {
		out = append(out, completioncore.Candidate{Text: name, Type: completioncore.CandidateColumn, Definition: owner})
	}
	return out
}

func derivedColumns(sql string, ref mssqlparser.RangeReference) []string {
	if len(ref.AliasColumns) > 0 {
		return ref.AliasColumns
	}
	if len(ref.Columns) > 0 {
		return ref.Columns
	}
	return projectedColumns(sql, ref.BodyLoc)
}

// projectedColumns names the select-list outputs of the query at loc: an
// explicit alias, else a column reference's column. For a set operation the
// left-most branch names the columns.
func projectedColumns(sql string, loc mssqlast.Loc) []string {
	if loc.Start < 0 || loc.End > len(sql) || loc.Start >= loc.End {
		return nil
	}
	body := strings.TrimSpace(sql[loc.Start:loc.End])
	if strings.HasPrefix(body, "(") && strings.HasSuffix(body, ")") {
		body = body[1 : len(body)-1]
	}
	list, err := mssqlparser.Parse(body)
	if err != nil || list == nil || len(list.Items) == 0 {
		return nil
	}
	stmt, ok := list.Items[0].(*mssqlast.SelectStmt)
	if !ok {
		return nil
	}
	for stmt.Op != mssqlast.SetOpNone && stmt.Larg != nil {
		stmt = stmt.Larg
	}
	if stmt.TargetList == nil {
		return nil
	}
	var names []string
	for _, item := range stmt.TargetList.Items {
		target, ok := item.(*mssqlast.ResTarget)
		if !ok {
			continue
		}
		name := target.Name
		if name == "" {
			if column, ok := target.Val.(*mssqlast.ColumnRef); ok {
				name = column.Column
			}
		}
		if name != "" && name != "*" {
			names = append(names, name)
		}
	}
	return names
}

func position(w wants, out []completioncore.Candidate) string {
	switch {
	case w.columns && hasType(out, completioncore.CandidateColumn):
		return completioncore.PositionColumn
	case w.relations && (hasType(out, completioncore.CandidateTable) || hasType(out, completioncore.CandidateView) || hasType(out, completioncore.CandidateForeignTable)):
		return completioncore.PositionRelation
	case onlyKeywords(out):
		return completioncore.PositionKeyword
	}
	return completioncore.PositionAny
}

func hasType(cands []completioncore.Candidate, t completioncore.CandidateType) bool {
	for _, c := range cands {
		if c.Type == t {
			return true
		}
	}
	return false
}

func onlyKeywords(cands []completioncore.Candidate) bool {
	if len(cands) == 0 {
		return false
	}
	for _, c := range cands {
		if c.Type != completioncore.CandidateKeyword {
			return false
		}
	}
	return true
}

// dedupe drops keywords that duplicate a function name and repeated
// candidates of one type, keeping the first, then orders by type and name.
func dedupe(cands []completioncore.Candidate) []completioncore.Candidate {
	functions := map[string]bool{}
	for _, c := range cands {
		if c.Type == completioncore.CandidateFunction {
			functions[strings.ToUpper(c.Text)] = true
		}
	}
	seen := map[string]bool{}
	out := make([]completioncore.Candidate, 0, len(cands))
	for _, c := range cands {
		if c.Type == completioncore.CandidateKeyword && functions[strings.ToUpper(c.Text)] {
			continue
		}
		key := string(c.Type) + "\x00" + strings.ToLower(c.Text)
		if c.Text == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return out[i].Type < out[j].Type
		}
		return strings.ToLower(out[i].Text) < strings.ToLower(out[j].Text)
	})
	return out
}

func filterByPrefix(cands []completioncore.Candidate, prefix string) []completioncore.Candidate {
	if prefix == "" {
		return cands
	}
	lower := strings.ToLower(prefix)
	out := make([]completioncore.Candidate, 0, len(cands))
	for _, c := range cands {
		if strings.HasPrefix(strings.ToLower(c.Text), lower) {
			out = append(out, c)
		}
	}
	return out
}
