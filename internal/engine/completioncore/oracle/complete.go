// Package oracle adapts Bytebase/Omni's Oracle parser-native completion to
// SQLWarden's dialect-neutral completion boundary. Omni exposes no Oracle
// catalog or high-level completion, so grammar candidates come from
// parser.CollectCompletion and every name is resolved from SQLWarden's own
// metadata index via completioncore.MetadataResolver.
package oracle

import (
	"context"
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

	cc := oracleparser.CollectCompletion(sql, cursor)
	prefix := cc.Prefix

	wantColumns, wantRelations, wantSchemas, wantRoutines := intentClasses(cc.Intent)
	qualifier := ""
	if cc.Intent != nil {
		qualifier = cc.Intent.Qualifier.Object
		if qualifier == "" {
			qualifier = cc.Intent.Qualifier.Schema
		}
	}

	var out []completioncore.Candidate

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

	if meta != nil {
		if wantColumns {
			out = append(out, columnCandidates(cc.Scope, qualifier, meta)...)
		}
		if wantRelations {
			out = append(out, relationCandidates(meta)...)
		}
		if wantSchemas {
			for _, schema := range meta.SchemaNames(meta.DefaultDatabase()) {
				out = append(out, completioncore.Candidate{Text: schema, Type: completioncore.CandidateSchema})
			}
		}
		if wantRoutines {
			out = append(out, routineCandidates(meta)...)
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
func intentClasses(intent *oracleparser.CompletionIntent) (columns, relations, schemas, routines bool) {
	if intent == nil || len(intent.ObjectKinds) == 0 {
		return false, true, true, false
	}
	for _, kind := range intent.ObjectKinds {
		switch kind {
		case oracleparser.ObjectKindColumn:
			columns = true
		case oracleparser.ObjectKindTable, oracleparser.ObjectKindView:
			relations = true
		case oracleparser.ObjectKindSchema, oracleparser.ObjectKindUser:
			schemas = true
		case oracleparser.ObjectKindFunction, oracleparser.ObjectKindProcedure, oracleparser.ObjectKindPackage:
			routines = true
		}
	}
	return columns, relations, schemas, routines
}

func columnCandidates(scope *oracleparser.ScopeSnapshot, qualifier string, meta completioncore.MetadataResolver) []completioncore.Candidate {
	if scope == nil {
		return nil
	}
	refs := append([]oracleparser.RangeReference{}, scope.LocalReferences...)
	for _, outer := range scope.OuterReferences { // nearest first
		refs = append(refs, outer...)
	}
	var out []completioncore.Candidate
	seen := map[string]bool{}
	for _, ref := range refs {
		if ref.Unsupported {
			continue
		}
		name := ref.Name
		if name == "" {
			name = ref.Alias
		}
		if qualifier != "" && !strings.EqualFold(qualifier, ref.Alias) && !strings.EqualFold(qualifier, ref.Name) {
			continue
		}
		relation, ok := meta.FindRelation(meta.DefaultDatabase(), ref.Schema, name)
		if !ok {
			// Subquery / CTE with parser-known columns.
			for _, col := range ref.Columns {
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

func relationCandidates(meta completioncore.MetadataResolver) []completioncore.Candidate {
	var out []completioncore.Candidate
	for _, relation := range meta.Relations(meta.DefaultDatabase(), meta.DefaultSchema()) {
		kind := relation.Kind
		if kind == "" {
			kind = completioncore.CandidateTable
		}
		out = append(out, completioncore.Candidate{Text: relation.Name, Type: kind, Definition: relation.Definition})
	}
	return out
}

func routineCandidates(meta completioncore.MetadataResolver) []completioncore.Candidate {
	var out []completioncore.Candidate
	for _, relation := range meta.Relations(meta.DefaultDatabase(), meta.DefaultSchema()) {
		if relation.Kind == completioncore.CandidateFunction || relation.Kind == completioncore.CandidateProcedure {
			out = append(out, completioncore.Candidate{Text: relation.Name, Type: relation.Kind})
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
	out := make([]completioncore.Candidate, 0, len(cands))
	for _, c := range cands {
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
		text := c.Text
		if dot := strings.LastIndexByte(text, '.'); dot >= 0 {
			text = text[dot+1:]
		}
		if strings.HasPrefix(strings.ToLower(text), lower) {
			out = append(out, c)
		}
	}
	return out
}
