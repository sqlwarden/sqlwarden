package postgres

import (
	"sync"

	omnipg "github.com/bytebase/omni/pg"
	"github.com/bytebase/omni/pg/catalog"
	"github.com/bytebase/omni/pg/parser"

	"github.com/sqlwarden/internal/engine/completioncore"
)

var relationRules = []string{"relation_expr", "qualified_name", "any_name"}

var builtinFunctionNames = sync.OnceValue(func() []string {
	return catalog.New().AllProcNames()
})

func resolveRuleCandidates(candidates *parser.CandidateSet, sql string, cursor int, metadata completioncore.MetadataResolver) []completioncore.Candidate {
	var out []completioncore.Candidate
	qualifier := qualifierAt(sql, cursor)
	if hasAnyRule(candidates, relationRules) && metadata != nil {
		out = append(out, relationRuleCandidates(qualifier, metadata)...)
	}
	if hasRule(candidates, "func_name") {
		out = append(out, functionRuleCandidates(qualifier, metadata)...)
	}
	return out
}

func ruleCandidatesAt(candidates *parser.CandidateSet, sql string, cursor int) *parser.CandidateSet {
	prefix := prefixAt(sql, cursor)
	if candidates != nil && len(candidates.Rules) == 0 && prefix != "" {
		ruleCursor := cursor - len(prefix)
		return omnipg.CollectCompletion(sql[:ruleCursor], ruleCursor).Candidates
	}
	return candidates
}

func relationRuleCandidates(qualifier string, metadata completioncore.MetadataResolver) []completioncore.Candidate {
	database := metadata.DefaultDatabase()
	namespace := qualifier
	var out []completioncore.Candidate
	if qualifier == "" {
		namespace = metadata.DefaultSchema()
		for _, schema := range metadata.SchemaNames(database) {
			out = append(out, completioncore.Candidate{Text: schema, Type: completioncore.CandidateSchema})
		}
	}
	for _, relation := range metadata.Relations(database, namespace) {
		candidate := completioncore.Candidate{Text: relation.Name, Type: relation.Kind, Definition: relation.Definition}
		if qualifier == "" && database == "" && relation.Schema != "" {
			candidate.Qualifier = []string{relation.Schema}
		}
		out = append(out, candidate)
	}
	if catalogResolver, ok := metadata.(completioncore.CatalogResolver); ok {
		for _, ref := range catalogResolver.CatalogObjects(database, namespace, "sequence") {
			out = append(out, completioncore.Candidate{Text: ref.Name, Type: completioncore.CandidateSequence})
		}
	}
	return out
}

func functionRuleCandidates(qualifier string, metadata completioncore.MetadataResolver) []completioncore.Candidate {
	var out []completioncore.Candidate
	if qualifier == "" {
		for _, name := range builtinFunctionNames() {
			out = append(out, completioncore.Candidate{Text: name, Type: completioncore.CandidateFunction})
		}
	}
	if catalogResolver, ok := metadata.(completioncore.CatalogResolver); ok {
		for _, ref := range catalogResolver.CatalogObjects(metadata.DefaultDatabase(), qualifier, "function") {
			out = append(out, completioncore.Candidate{Text: ref.Name, Type: completioncore.CandidateFunction})
		}
	}
	return out
}

func hasAnyRule(candidates *parser.CandidateSet, rules []string) bool {
	for _, rule := range rules {
		if hasRule(candidates, rule) {
			return true
		}
	}
	return false
}
