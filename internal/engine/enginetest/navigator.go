package enginetest

import (
	"context"
	"reflect"
	"testing"

	"github.com/sqlwarden/internal/engine/metadata"
)

type NavigatorCase struct {
	NodeKind string
	Folder   string
	Parents  []metadata.ScopePath
}

// RunNavigatorContract asserts that each folder loader, given several parents
// at once, returns only requested parents and the same children it returns for
// each parent alone.
func RunNavigatorContract(t *testing.T, inspector metadata.SchemaInspector, database string, cases []NavigatorCase) {
	t.Helper()
	tree := inspector.Tree()
	if err := tree.Validate(); err != nil {
		t.Fatalf("navigator tree invalid: %v", err)
	}
	ctx := context.Background()
	q, err := inspector.Querier(ctx, database)
	if err != nil {
		t.Fatalf("Querier(%q): %v", database, err)
	}
	for _, c := range cases {
		folder, ok := tree.Folder(c.NodeKind, c.Folder)
		if !ok {
			t.Fatalf("folder %q under %q is not declared", c.Folder, c.NodeKind)
		}
		batch, err := folder.List(ctx, q, c.Parents)
		if err != nil {
			t.Fatalf("%s/%s batch: %v", c.NodeKind, c.Folder, err)
		}
		requested := map[metadata.ScopePath]bool{}
		for _, p := range c.Parents {
			requested[p] = true
		}
		for key := range batch {
			if !requested[key] {
				t.Fatalf("%s/%s returned unrequested parent %q", c.NodeKind, c.Folder, key)
			}
		}
		for _, p := range c.Parents {
			single, err := folder.List(ctx, q, []metadata.ScopePath{p})
			if err != nil {
				t.Fatalf("%s/%s single %q: %v", c.NodeKind, c.Folder, p, err)
			}
			if len(single[p]) == 0 && len(batch[p]) == 0 {
				continue
			}
			if !reflect.DeepEqual(single[p], batch[p]) {
				t.Fatalf("%s/%s parent %q: batch %v != single %v", c.NodeKind, c.Folder, p, batch[p], single[p])
			}
			for _, child := range batch[p] {
				if !folder.Contains(child.Kind) {
					t.Fatalf("%s/%s returned kind %q outside the folder", c.NodeKind, c.Folder, child.Kind)
				}
			}
		}
	}
}
