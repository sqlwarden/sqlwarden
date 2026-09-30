package tidb

import (
	"strings"

	"github.com/sqlwarden/internal/engine/engines/mysql"
	"github.com/sqlwarden/internal/engine/metadata"
)

var _ metadata.SchemaInspector = (*driver)(nil)

var systemDatabases = map[string]bool{"metrics_schema": true}

func (d *driver) Tree() metadata.Tree {
	return navigatorTree
}

var navigatorTree = func() metadata.Tree {
	tree := (&mysql.Driver{}).Tree().
		WithoutFolder("database", "procedures").
		WithoutFolder("database", "triggers").
		WithoutFolder("database", "events").
		WithoutFolder("table", "triggers").
		WithNode("sequence", metadata.Node{Label: "Sequence", Icon: "sequence", Leaf: true, HasDefinition: true}).
		WithFolder("database", metadata.Folder{Kind: "sequences", Label: "Sequences", Child: "sequence", Order: 70, List: mysql.ListSequences})
	databases, _ := tree.Folder("", "databases")
	databases.List = metadata.MarkSystem(databases.List, func(c metadata.Child) bool { return systemDatabases[strings.ToLower(c.Name)] })
	return tree.WithFolder("", databases)
}()
