package yugabyte

import (
	"github.com/sqlwarden/internal/engine/engines/postgres"
	"github.com/sqlwarden/internal/engine/metadata"
)

var _ metadata.SchemaInspector = (*driver)(nil)

var systemDatabases = map[string]bool{"system_platform": true}

func (d *driver) Tree() metadata.Tree {
	return navigatorTree
}

var navigatorTree = func() metadata.Tree {
	tree := (&postgres.Driver{}).Tree()
	databases, _ := tree.Folder("", "databases")
	databases.List = metadata.MarkSystem(databases.List, func(c metadata.Child) bool { return systemDatabases[c.Name] })
	return tree.WithFolder("", databases)
}()
