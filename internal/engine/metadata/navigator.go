package metadata

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
)

// Querier is the narrow query surface navigator loaders run against.
// *sql.DB satisfies it.
type Querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Loader lists a folder's children for a batch of parents and returns them
// grouped by parent path. Parents always share a node kind and database.
// Parents with no children may be absent from the result.
type Loader func(ctx context.Context, q Querier, parents []ScopePath) (map[ScopePath][]Child, error)

type Child struct {
	Kind       string         `json:"kind"`
	Name       string         `json:"name"`
	Attributes map[string]any `json:"attributes,omitempty"`
	System     bool           `json:"system"`
	Current    bool           `json:"current"`
}

// Folder is a grammar-only grouping under a node. It never appears in paths;
// a listing is addressed by (parent path, folder kind).
type Folder struct {
	Kind       string   `json:"kind"`
	Label      string   `json:"label"`
	Child      string   `json:"child"`
	MixedKinds []string `json:"mixed_kinds,omitempty"`
	Order      int      `json:"order"`
	List       Loader   `json:"-"`
}

func (f Folder) Contains(kind string) bool {
	return f.Child == kind || slices.Contains(f.MixedKinds, kind)
}

func (f Folder) kinds() []string {
	return append([]string{f.Child}, f.MixedKinds...)
}

type Node struct {
	Label            string   `json:"label"`
	Icon             string   `json:"icon"`
	Leaf             bool     `json:"leaf"`
	Folders          []Folder `json:"folders"`
	Scope            bool     `json:"scope"`
	Relational       bool     `json:"relational"`
	SupportsDiagram  bool     `json:"supports_diagram"`
	HasDefinition    bool     `json:"has_definition"`
	ShowAllDatabases bool     `json:"show_all_databases"`
	Column           bool     `json:"column"`
}

func (n Node) MarshalJSON() ([]byte, error) {
	type plain Node
	sorted := plain(n)
	sorted.Folders = slices.Clone(n.Folders)
	if sorted.Folders == nil {
		sorted.Folders = []Folder{}
	}
	sort.SliceStable(sorted.Folders, func(i, j int) bool { return sorted.Folders[i].Order < sorted.Folders[j].Order })
	return json.Marshal(sorted)
}

// Tree is a driver's static navigator grammar. It must not touch the target
// database.
type Tree struct {
	Root          Node            `json:"root"`
	Nodes         map[string]Node `json:"nodes"`
	SystemObjects bool            `json:"system_objects"`
}

var KnownIcons = map[string]bool{
	"connection": true, "database": true, "schema": true, "table": true, "view": true,
	"materialized_view": true, "foreign_table": true, "column": true, "constraint": true,
	"foreign_key": true, "index": true, "dependency": true, "reference": true,
	"partition": true, "trigger": true, "rule": true, "policy": true, "function": true,
	"procedure": true, "sequence": true, "type": true, "domain": true, "aggregate": true,
	"event_trigger": true, "extension": true, "event": true, "user": true, "role": true,
	"profile": true, "package": true, "queue": true, "synonym": true, "extended_property": true,
}

func (t Tree) Node(kind string) (Node, bool) {
	if kind == "" {
		return t.Root, true
	}
	node, ok := t.Nodes[kind]
	return node, ok
}

func (t Tree) Folder(nodeKind, folderKind string) (Folder, bool) {
	node, ok := t.Node(nodeKind)
	if !ok {
		return Folder{}, false
	}
	for _, folder := range node.Folders {
		if folder.Kind == folderKind {
			return folder, true
		}
	}
	return Folder{}, false
}

// DatabaseKind returns the node kind "Show all databases" filters, or "".
func (t Tree) DatabaseKind() string {
	for kind, node := range t.Nodes {
		if node.ShowAllDatabases {
			return kind
		}
	}
	return ""
}

// DatabaseOf returns the database a path belongs to, or "" for the connection
// default.
func (t Tree) DatabaseOf(path ScopePath) string {
	kind := t.DatabaseKind()
	if kind == "" {
		return ""
	}
	return path.Name(kind)
}

func (t Tree) NodeKindOf(path ScopePath) string {
	last, ok := path.Last()
	if !ok {
		return ""
	}
	return last.Kind
}

// FoldersContaining returns the folders under parentKind whose items may be
// of childKind, in declaration order.
func (t Tree) FoldersContaining(parentKind, childKind string) []Folder {
	node, ok := t.Node(parentKind)
	if !ok {
		return nil
	}
	var out []Folder
	for _, folder := range node.Folders {
		if folder.Contains(childKind) {
			out = append(out, folder)
		}
	}
	return out
}

func (t Tree) Validate() error {
	var errs []error
	databaseKinds := 0
	check := func(kind string, node Node) {
		if !KnownIcons[node.Icon] {
			errs = append(errs, fmt.Errorf("node %q: unknown icon %q", kind, node.Icon))
		}
		if node.Leaf && len(node.Folders) > 0 {
			errs = append(errs, fmt.Errorf("node %q: leaf declares folders", kind))
		}
		if node.ShowAllDatabases {
			databaseKinds++
		}
		seen := map[string]bool{}
		for _, folder := range node.Folders {
			if folder.Kind == "" || folder.Label == "" {
				errs = append(errs, fmt.Errorf("node %q: folder kind and label are required", kind))
			}
			if seen[folder.Kind] {
				errs = append(errs, fmt.Errorf("node %q: duplicate folder %q", kind, folder.Kind))
			}
			seen[folder.Kind] = true
			if folder.List == nil {
				errs = append(errs, fmt.Errorf("node %q folder %q: nil loader", kind, folder.Kind))
			}
			for _, child := range folder.kinds() {
				if _, ok := t.Nodes[child]; !ok {
					errs = append(errs, fmt.Errorf("node %q folder %q: undeclared child kind %q", kind, folder.Kind, child))
				}
			}
		}
	}
	check("", t.Root)
	for kind, node := range t.Nodes {
		check(kind, node)
	}
	if databaseKinds > 1 {
		errs = append(errs, errors.New("more than one node kind sets ShowAllDatabases"))
	}
	return errors.Join(errs...)
}

func (t Tree) clone() Tree {
	out := Tree{Root: cloneNode(t.Root), Nodes: make(map[string]Node, len(t.Nodes)), SystemObjects: t.SystemObjects}
	for kind, node := range t.Nodes {
		out.Nodes[kind] = cloneNode(node)
	}
	return out
}

func cloneNode(node Node) Node {
	node.Folders = slices.Clone(node.Folders)
	for i := range node.Folders {
		node.Folders[i].MixedKinds = slices.Clone(node.Folders[i].MixedKinds)
	}
	return node
}

func (t Tree) WithNode(kind string, node Node) Tree {
	out := t.clone()
	if kind == "" {
		out.Root = cloneNode(node)
	} else {
		out.Nodes[kind] = cloneNode(node)
	}
	return out
}

// WithFolder replaces the folder of the same kind under nodeKind, or appends it.
func (t Tree) WithFolder(nodeKind string, folder Folder) Tree {
	out := t.clone()
	node, _ := out.Node(nodeKind)
	replaced := false
	for i := range node.Folders {
		if node.Folders[i].Kind == folder.Kind {
			node.Folders[i] = folder
			replaced = true
		}
	}
	if !replaced {
		node.Folders = append(node.Folders, folder)
	}
	return out.WithNode(nodeKind, node)
}

func (t Tree) WithoutFolder(nodeKind, folderKind string) Tree {
	out := t.clone()
	node, _ := out.Node(nodeKind)
	node.Folders = slices.DeleteFunc(node.Folders, func(f Folder) bool { return f.Kind == folderKind })
	return out.WithNode(nodeKind, node)
}

// MarkSystem wraps list so children matching system are flagged System. It
// never clears a flag the wrapped loader already set.
func MarkSystem(list Loader, system func(Child) bool) Loader {
	return func(ctx context.Context, q Querier, parents []ScopePath) (map[ScopePath][]Child, error) {
		out, err := list(ctx, q, parents)
		if err != nil {
			return nil, err
		}
		for _, children := range out {
			for i := range children {
				if system(children[i]) {
					children[i].System = true
				}
			}
		}
		return out, nil
	}
}
