package mysql

import (
	"github.com/bytebase/omni/mysql/parser"

	"github.com/sqlwarden/internal/engine/completioncore"
)

// isRelationSlot reports whether the cursor sits in a relation-name slot
// (INSERT INTO |, UPDATE |, DROP TABLE |) of a statement that otherwise counts
// as a column context, which is the case for every DML statement as a whole.
func isRelationSlot(sql string, cursor int) bool {
	rules := parser.Collect(sql, cursor-len(prefixAt(sql, cursor)))
	return (hasRule(rules, "table_ref") || hasRule(rules, "view_ref")) && !hasRule(rules, "columnref")
}

func resolveRuleCandidates(sql string, cursor int, metadata completioncore.MetadataResolver) []completioncore.Candidate {
	if metadata == nil {
		return nil
	}
	rules := parser.Collect(sql, cursor-len(prefixAt(sql, cursor)))
	qualifier := qualifierAt(sql, cursor)
	var out []completioncore.Candidate
	if hasRule(rules, "table_ref") || hasRule(rules, "view_ref") {
		database := qualifier
		if database == "" {
			database = metadata.DefaultDatabase()
		}
		for _, relation := range metadata.Relations("", database) {
			candidate := completioncore.Candidate{Text: relation.Name, Type: relation.Kind, Definition: relation.Definition}
			if qualifier == "" && database == "" && relation.Database != "" {
				candidate.Qualifier = []string{relation.Database}
			}
			out = append(out, candidate)
		}
	}
	if hasRule(rules, "database_ref") && qualifier == "" {
		for _, name := range metadata.DatabaseNames() {
			out = append(out, completioncore.Candidate{Text: name, Type: completioncore.CandidateDatabase})
		}
	}
	return out
}
