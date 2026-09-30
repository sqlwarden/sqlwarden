package mariadb

import (
	"context"

	"github.com/sqlwarden/internal/engine/engines/mysql"
	"github.com/sqlwarden/internal/engine/metadata"
)

var _ metadata.SchemaInspector = (*driver)(nil)

func (d *driver) Tree() metadata.Tree {
	return navigatorTree
}

var navigatorTree = func() metadata.Tree {
	tree := (&mysql.Driver{}).Tree().
		WithNode("sequence", metadata.Node{Label: "Sequence", Icon: "sequence", Leaf: true, HasDefinition: true}).
		WithFolder("database", metadata.Folder{Kind: "sequences", Label: "Sequences", Child: "sequence", Order: 70, List: mysql.ListSequences})
	for _, kind := range []string{"table", "view"} {
		columns, _ := tree.Folder(kind, "columns")
		columns.List = declareJSONColumns(columns.List)
		tree = tree.WithFolder(kind, columns)
	}
	users, _ := tree.Folder("", "users")
	users.List = metadata.MarkSystem(users.List, func(c metadata.Child) bool { return c.Attributes["user"] == "mariadb.sys" })
	return tree.WithFolder("", users)
}()

// declareJSONColumns reports "json" for columns MariaDB stores as longtext
// behind its auto-generated json_valid CHECK; see jsonColumns.
func declareJSONColumns(list metadata.Loader) metadata.Loader {
	return func(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
		out, err := list(ctx, q, parents)
		if err != nil || len(parents) == 0 {
			return out, err
		}
		refs := make([]metadata.ObjectRef, 0, len(parents))
		for _, p := range parents {
			if ref, ok := metadata.ObjectRefOf(p); ok {
				refs = append(refs, ref)
			}
		}
		declared, err := jsonColumns(ctx, q, refs)
		if err != nil {
			return nil, err
		}
		for parent, children := range out {
			ref, _ := metadata.ObjectRefOf(parent)
			for i := range children {
				if declared[ref.Scope.Name("database")+"\x00"+ref.Name+"\x00"+children[i].Name] {
					children[i].Attributes["data_type"] = "json"
				}
			}
		}
		return out, nil
	}
}
