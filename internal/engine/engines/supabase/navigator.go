package supabase

import (
	"github.com/sqlwarden/internal/engine/engines/postgres"
	"github.com/sqlwarden/internal/engine/metadata"
)

var _ metadata.SchemaInspector = (*driver)(nil)

func (d *driver) Tree() metadata.Tree {
	return navigatorTree
}

var navigatorTree = func() metadata.Tree {
	tree := (&postgres.Driver{}).Tree()
	schemas, _ := tree.Folder("database", "schemas")
	schemas.List = metadata.MarkSystem(schemas.List, func(c metadata.Child) bool { return managedSchemas[c.Name] })
	return tree.WithFolder("database", schemas)
}()
