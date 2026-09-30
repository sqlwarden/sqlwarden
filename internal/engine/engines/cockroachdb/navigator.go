package cockroachdb

import (
	"github.com/sqlwarden/internal/engine/engines/postgres"
	"github.com/sqlwarden/internal/engine/metadata"
)

var _ metadata.SchemaInspector = (*driver)(nil)

var systemDatabases = map[string]bool{"system": true}

func (d *driver) Tree() metadata.Tree {
	return navigatorTree
}

var navigatorTree = func() metadata.Tree {
	tree := (&postgres.Driver{}).Tree()
	for _, f := range [][2]string{
		{"database", "event_triggers"}, {"database", "extensions"},
		{"schema", "foreign_tables"}, {"schema", "aggregate_functions"},
		{"table", "partitions"}, {"table", "rules"}, {"view", "rules"},
	} {
		tree = tree.WithoutFolder(f[0], f[1])
	}
	databases, _ := tree.Folder("", "databases")
	databases.List = metadata.MarkSystem(databases.List, func(c metadata.Child) bool { return systemDatabases[c.Name] })
	schemas, _ := tree.Folder("database", "schemas")
	schemas.List = metadata.MarkSystem(schemas.List, func(c metadata.Child) bool { return systemSchemas[c.Name] })
	return tree.WithFolder("", databases).WithFolder("database", schemas)
}()
