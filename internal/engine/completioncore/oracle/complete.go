// Package oracle adapts Bytebase/Omni's Oracle parser-native completion to
// SQLWarden's dialect-neutral completion boundary. Omni exposes no Oracle
// catalog or high-level completion, so grammar candidates come from
// parser.CollectCompletion and every name is resolved from SQLWarden's own
// metadata index via completioncore.MetadataResolver.
package oracle

import (
	"context"
	"slices"
	"sort"
	"strings"
	"unicode"

	oracleparser "github.com/bytebase/omni/oracle/parser"

	"github.com/sqlwarden/internal/engine/completioncore"
)

func Complete(
	ctx context.Context,
	sql string,
	cursor int,
	meta completioncore.MetadataResolver,
) ([]completioncore.Candidate, completioncore.Context, error) {
	if err := completioncore.CheckContext(ctx); err != nil {
		return nil, completioncore.Context{}, err
	}
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(sql) {
		cursor = len(sql)
	}

	_, _, prefix, suppressed := CursorWord(sql, cursor)
	if suppressed {
		return nil, completioncore.Context{}, nil
	}
	cc := oracleparser.CollectCompletion(sql, cursor)

	wantColumns, wantRelations, wantSchemas := intentClasses(cc.Intent)
	qualifier := ""
	if cc.Intent != nil {
		qualifier = cc.Intent.Qualifier.Object
		if qualifier == "" {
			qualifier = cc.Intent.Qualifier.Schema
		}
	}

	var out []completioncore.Candidate
	wantFunctions := wantsKind(cc.Intent, oracleparser.ObjectKindFunction)
	if wantColumns && qualifier == "" {
		start, _, _, _ := CursorWord(sql, cursor)
		tokens := oracleparser.Tokenize(sql[:start])
		if len(tokens) > 0 {
			switch tokens[len(tokens)-1].Type {
			case oracleparser.WHERE, oracleparser.ON, oracleparser.BY:
				wantFunctions = true
			}
		}
	}
	if wantFunctions && wantColumns && qualifier == "" {
		for _, function := range builtinFunctions {
			out = append(out, completioncore.Candidate{
				Text: function.Name, InsertText: function.Name,
				Type: completioncore.CandidateFunction, Definition: function.Detail,
			})
		}
	}

	// Grammar / keyword candidates.
	if cc.Candidates != nil {
		for _, tok := range cc.Candidates.Tokens {
			if name := oracleparser.TokenName(tok); name != "" && isWord(name) {
				out = append(out, completioncore.Candidate{
					Text: strings.ToUpper(name), Type: completioncore.CandidateKeyword,
				})
			}
		}
	}

	if wantRelations && qualifier == "" {
		for _, cte := range cc.CTEs {
			name := parserIdentifier(cte.Name, sql[cte.Loc.Start:cte.Loc.End])
			out = append(out, completioncore.Candidate{Text: name, Type: completioncore.CandidateTable, Definition: "Common table expression"})
		}
	}
	if wantColumns {
		out = append(out, columnCandidates(cc.Scope, qualifier, meta, sql)...)
	}
	if meta != nil {
		if wantRelations {
			out = append(out, relationCandidates(meta, qualifier)...)
		}
		if wantSchemas {
			for _, schema := range meta.SchemaNames(meta.DefaultDatabase()) {
				out = append(out, completioncore.Candidate{Text: schema, Type: completioncore.CandidateSchema})
			}
		}
		if catalog, ok := meta.(completioncore.CatalogResolver); ok {
			kinds := []string{}
			schemaQualifier := false
			if qualifier != "" && wantColumns {
				for _, schema := range meta.SchemaNames(meta.DefaultDatabase()) {
					if strings.EqualFold(schema, qualifier) {
						schemaQualifier = true
						break
					}
				}
			}
			if schemaQualifier {
				kinds = append(kinds, "function", "sequence")
			}
			if wantFunctions {
				kinds = append(kinds, "function")
			}
			if wantsKind(cc.Intent, oracleparser.ObjectKindProcedure) {
				kinds = append(kinds, "procedure")
			}
			if wantsKind(cc.Intent, oracleparser.ObjectKindSequence) || (wantFunctions && qualifier == "") {
				kinds = append(kinds, "sequence")
			}
			for _, object := range catalog.CatalogObjects(meta.DefaultDatabase(), qualifier, kinds...) {
				out = append(out, completioncore.Candidate{Text: object.Name, Type: completioncore.CandidateType(object.Kind), Definition: object.Scope.Name("schema") + " · " + object.Kind})
			}
			if qualifier != "" && wantsKind(cc.Intent, oracleparser.ObjectKindSequenceMember) {
				schema := cc.Intent.Qualifier.Schema
				for _, sequence := range catalog.CatalogObjects(meta.DefaultDatabase(), schema, "sequence") {
					if strings.EqualFold(sequence.Name, qualifier) {
						for _, member := range []string{"NEXTVAL", "CURRVAL"} {
							out = append(out, completioncore.Candidate{Text: member, InsertText: member, Type: completioncore.CandidateKeyword, Definition: "Sequence value"})
						}
					}
				}
			}
		}
	}

	out = filterByPrefix(dedupe(out), prefix)

	position := completioncore.PositionAny
	switch {
	case wantColumns && hasType(out, completioncore.CandidateColumn):
		position = completioncore.PositionColumn
	case wantRelations && hasType(out, completioncore.CandidateTable):
		position = completioncore.PositionRelation
	case onlyKeywords(out):
		position = completioncore.PositionKeyword
	}

	return out, completioncore.Context{Position: position}, completioncore.CheckContext(ctx)
}

// intentClasses maps omni ObjectKinds onto the catalog classes to emit. With no
// intent, default to relations + keywords (a conservative FROM-ish position).
func intentClasses(intent *oracleparser.CompletionIntent) (columns, relations, schemas bool) {
	if intent == nil || len(intent.ObjectKinds) == 0 {
		return false, true, true
	}
	for _, kind := range intent.ObjectKinds {
		switch kind {
		case oracleparser.ObjectKindColumn:
			columns = true
		case oracleparser.ObjectKindTable, oracleparser.ObjectKindView:
			relations = true
		case oracleparser.ObjectKindSchema, oracleparser.ObjectKindUser:
			schemas = true
		}
	}
	return columns, relations, schemas
}

func wantsKind(intent *oracleparser.CompletionIntent, kind oracleparser.ObjectKind) bool {
	return intent != nil && slices.Contains(intent.ObjectKinds, kind)
}

func columnCandidates(scope *oracleparser.ScopeSnapshot, qualifier string, meta completioncore.MetadataResolver, sql string) []completioncore.Candidate {
	if scope == nil {
		return nil
	}
	refs := append([]oracleparser.RangeReference{}, scope.LocalReferences...)
	for _, outer := range scope.OuterReferences { // nearest first
		refs = append(refs, outer...)
	}
	var out []completioncore.Candidate
	seen := map[string]bool{}
	seenRefs := map[string]bool{}
	for _, ref := range refs {
		if ref.Unsupported {
			continue
		}
		visibleName := ref.Alias
		if visibleName == "" {
			visibleName = ref.Name
		}
		refKey := strings.ToUpper(visibleName)
		if visibleName != "" && seenRefs[refKey] {
			continue
		}
		if visibleName != "" {
			seenRefs[refKey] = true
		}
		name := ref.Name
		if name == "" {
			name = ref.Alias
		}
		if qualifier != "" && !strings.EqualFold(qualifier, visibleName) {
			continue
		}
		var relation completioncore.Relation
		var ok bool
		if meta != nil && ref.Kind != oracleparser.RangeReferenceCTE && ref.Kind != oracleparser.RangeReferenceSubquery && ref.Kind != oracleparser.RangeReferenceJoinAlias {
			relation, ok = meta.FindRelation(meta.DefaultDatabase(), ref.Schema, name)
		}
		if !ok {
			// Subquery / CTE with parser-known columns.
			for _, col := range ref.Columns {
				col = parserIdentifier(col, sql)
				if col != "" && col != "*" && !seen[strings.ToUpper(col)] {
					seen[strings.ToUpper(col)] = true
					out = append(out, completioncore.Candidate{Text: col, Type: completioncore.CandidateColumn})
				}
			}
			continue
		}
		for _, col := range relation.Columns {
			key := strings.ToUpper(col.Name)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, completioncore.Candidate{
				Text: col.Name, Type: completioncore.CandidateColumn,
				Definition: completioncore.ColumnDefinition(relation, col), Comment: col.Comment,
			})
		}
	}
	return out
}

func relationCandidates(meta completioncore.MetadataResolver, qualifier string) []completioncore.Candidate {
	schema := meta.DefaultSchema()
	if qualifier != "" {
		schema = qualifier
	}
	var out []completioncore.Candidate
	for _, relation := range meta.Relations(meta.DefaultDatabase(), schema) {
		kind := relation.Kind
		if kind == "" {
			kind = completioncore.CandidateTable
		}
		out = append(out, completioncore.Candidate{Text: relation.Name, Type: kind, Definition: relation.Definition})
	}
	if catalog, ok := meta.(completioncore.CatalogResolver); ok {
		for _, ref := range catalog.CatalogObjects(meta.DefaultDatabase(), schema, "table", "view", "materialized_view") {
			out = append(out, completioncore.Candidate{Text: ref.Name, Type: completioncore.CandidateType(ref.Kind)})
		}
	}
	return out
}

func isWord(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) && r != '_' {
			return false
		}
	}
	return s != ""
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

func dedupe(cands []completioncore.Candidate) []completioncore.Candidate {
	seen := map[string]bool{}
	functions := map[string]bool{}
	for _, candidate := range cands {
		if candidate.Type == completioncore.CandidateFunction {
			functions[strings.ToUpper(candidate.Text)] = true
		}
	}
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

// Omni folds bare names to lower case; Oracle folds them to upper case.
// Quoted names retain the source spelling.
func parserIdentifier(name, sql string) string {
	for _, token := range oracleparser.Tokenize(sql) {
		if token.Str == name && token.Loc < len(sql) && sql[token.Loc] == '"' {
			return name
		}
	}
	return strings.ToUpper(name)
}
