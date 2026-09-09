package schema

import (
	"context"
	"github.com/sqlwarden/internal/engine/metadata"
	"testing"
	"time"
)

type scopedInspector struct{ fakeSchemaInspector }

func (f *scopedInspector) InspectDirectory(_ context.Context, opts metadata.DirectoryOptions) (*metadata.Directory, error) {
	f.directoryHits++
	return &metadata.Directory{GeneratedAt: time.Now(), Roots: []metadata.ScopeNode{{Path: opts.Root, Groups: []metadata.ObjectGroup{{Kind: "table", Objects: []metadata.ObjectRef{{Scope: opts.Root, Kind: "table", Name: "orders"}}}}}}}, nil
}

func TestScopedDirectoriesOverlayAndRefresh(t *testing.T) {
	s := newService()
	inspector := &scopedInspector{}
	ctx := context.Background()
	a := metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "A"})
	b := metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "B"})
	base := &metadata.Directory{Roots: []metadata.ScopeNode{{Path: a, Lazy: true, System: true}, {Path: b, Lazy: true}}}
	for _, entry := range []struct {
		connection string
		scope      metadata.ScopePath
	}{{"1", a}, {"1", b}, {"10", a}, {"1", a}} {
		if _, err := s.DirectoryInScope(ctx, entry.connection, entry.scope, inspector); err != nil {
			t.Fatal(err)
		}
	}
	if inspector.directoryHits != 3 {
		t.Fatalf("scope cache misses: %d", inspector.directoryHits)
	}
	merged := s.WithCachedScopes("1", base)
	if len(merged.ObjectRefs()) != 2 || merged.Roots[0].Lazy || !merged.Roots[0].System || merged.GeneratedAt.IsZero() {
		t.Fatalf("bad overlay: %+v", merged)
	}
	if !base.Roots[0].Lazy || len(base.ObjectRefs()) != 0 {
		t.Fatal("snapshot mutated")
	}
	s.RefreshConnection("1")
	if _, ok := s.CachedScopeDirectory("1", a); ok {
		t.Fatal("scope survived refresh")
	}
	if _, ok := s.CachedScopeDirectory("1", b); ok {
		t.Fatal("second scope survived refresh")
	}
	if _, ok := s.CachedScopeDirectory("10", a); !ok {
		t.Fatal("refresh crossed connection boundary")
	}
	if !s.WithCachedScopes("1", base).Roots[0].Lazy {
		t.Fatal("stale overlay after refresh")
	}
}
