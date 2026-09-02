package oracle

import "github.com/sqlwarden/internal/engine/metadata"

func oracleSchemaScope(name string) metadata.ScopePath {
	return metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: name})
}

func oracleTestTable() metadata.Object {
	return metadata.Object{
		Ref: metadata.ObjectRef{
			Scope: oracleSchemaScope("HR"),
			Kind:  "table",
			Name:  "EMPLOYEES",
		},
		Relational: &metadata.RelationalDetail{
			Columns: []metadata.Column{{Name: "ID"}, {Name: "NAME"}},
		},
	}
}
