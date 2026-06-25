package schema

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sqlwarden/internal/cache"
	schemameta "github.com/sqlwarden/internal/dbengine/schema"
)

type fakeSchemaInspector struct {
	mu          sync.Mutex
	catalogHits int32
	objectCalls int32
	objectRefs  [][]schemameta.ObjectRef
	delay       time.Duration
}

func (f *fakeSchemaInspector) SchemaSpec() schemameta.SchemaSpec {
	return schemameta.SchemaSpec{Dialect: "fake", Kinds: []schemameta.SchemaObjectKind{{Kind: "table"}}}
}

func (f *fakeSchemaInspector) InspectCatalog(ctx context.Context, opts schemameta.CatalogOptions) (*schemameta.Catalog, error) {
	atomic.AddInt32(&f.catalogHits, 1)
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	return &schemameta.Catalog{
		Dialect: "fake",
		Namespaces: []schemameta.NamespaceCatalog{{Name: "public", Groups: []schemameta.ObjectGroupCatalog{{
			Kind:    "table",
			Objects: []schemameta.ObjectRef{{Namespace: "public", Kind: "table", Name: "users"}},
		}}}},
	}, nil
}

func (f *fakeSchemaInspector) InspectObjects(ctx context.Context, refs []schemameta.ObjectRef) ([]schemameta.Object, error) {
	atomic.AddInt32(&f.objectCalls, 1)
	f.mu.Lock()
	f.objectRefs = append(f.objectRefs, refs)
	f.mu.Unlock()
	out := make([]schemameta.Object, 0, len(refs))
	for _, r := range refs {
		out = append(out, schemameta.Object{Ref: r, Relational: &schemameta.RelationalDetail{Columns: []schemameta.Column{{Name: "id"}}}})
	}
	return out, nil
}

func newService() *Service { return NewService(cache.NewMemCache(64), time.Minute) }

func TestServiceCatalogCachesAfterMiss(t *testing.T) {
	s := newService()
	intr := &fakeSchemaInspector{}
	if _, err := s.Catalog(context.Background(), "c1", intr); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Catalog(context.Background(), "c1", intr); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&intr.catalogHits); got != 1 {
		t.Fatalf("want 1 inspection, got %d", got)
	}
}

func TestServiceCatalogSingleflight(t *testing.T) {
	s := newService()
	intr := &fakeSchemaInspector{delay: 50 * time.Millisecond}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = s.Catalog(context.Background(), "c1", intr) }()
	}
	wg.Wait()
	if got := atomic.LoadInt32(&intr.catalogHits); got != 1 {
		t.Fatalf("singleflight should collapse to 1, got %d", got)
	}
}

func TestServiceObjectsFetchesOnlyMissing(t *testing.T) {
	s := newService()
	intr := &fakeSchemaInspector{}
	ctx := context.Background()
	users := schemameta.ObjectRef{Namespace: "public", Kind: "table", Name: "users"}
	orders := schemameta.ObjectRef{Namespace: "public", Kind: "table", Name: "orders"}

	if _, err := s.Objects(ctx, "c1", []schemameta.ObjectRef{users}, intr); err != nil {
		t.Fatal(err)
	}
	// users now cached; requesting users+orders must fetch only orders.
	got, err := s.Objects(ctx, "c1", []schemameta.ObjectRef{users, orders}, intr)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Ref != users || got[1].Ref != orders {
		t.Fatalf("expected [users, orders] in request order, got %+v", got)
	}
	intr.mu.Lock()
	defer intr.mu.Unlock()
	if len(intr.objectRefs) != 2 {
		t.Fatalf("want 2 driver calls, got %d", len(intr.objectRefs))
	}
	if len(intr.objectRefs[1]) != 1 || intr.objectRefs[1][0] != orders {
		t.Fatalf("second call should fetch only orders, got %+v", intr.objectRefs[1])
	}
}

func TestServiceRefreshObjectVsConnection(t *testing.T) {
	s := newService()
	intr := &fakeSchemaInspector{}
	ctx := context.Background()
	users := schemameta.ObjectRef{Namespace: "public", Kind: "table", Name: "users"}

	_, _ = s.Catalog(ctx, "c1", intr)
	_, _ = s.Objects(ctx, "c1", []schemameta.ObjectRef{users}, intr)

	// RefreshObject drops only the object; catalog stays cached.
	s.RefreshObject("c1", users)
	_, _ = s.Objects(ctx, "c1", []schemameta.ObjectRef{users}, intr)
	_, _ = s.Catalog(ctx, "c1", intr)
	if got := atomic.LoadInt32(&intr.objectCalls); got != 2 {
		t.Fatalf("want 2 object fetches after RefreshObject, got %d", got)
	}
	if got := atomic.LoadInt32(&intr.catalogHits); got != 1 {
		t.Fatalf("catalog should still be cached, got %d hits", got)
	}

	// RefreshConnection drops catalog + all object detail.
	s.RefreshConnection("c1")
	_, _ = s.Catalog(ctx, "c1", intr)
	_, _ = s.Objects(ctx, "c1", []schemameta.ObjectRef{users}, intr)
	if got := atomic.LoadInt32(&intr.catalogHits); got != 2 {
		t.Fatalf("catalog should re-inspect after RefreshConnection, got %d", got)
	}
	if got := atomic.LoadInt32(&intr.objectCalls); got != 3 {
		t.Fatalf("object should re-fetch after RefreshConnection, got %d", got)
	}
}

type erroringSchemaInspector struct{ fakeSchemaInspector }

func (e *erroringSchemaInspector) InspectCatalog(ctx context.Context, opts schemameta.CatalogOptions) (*schemameta.Catalog, error) {
	return nil, context.DeadlineExceeded
}

func TestServiceDoesNotCacheCatalogError(t *testing.T) {
	s := newService()
	intr := &erroringSchemaInspector{}
	if _, err := s.Catalog(context.Background(), "c1", intr); err == nil {
		t.Fatal("expected error")
	}
	if _, ok := s.cache.Get(catalogKey("c1")); ok {
		t.Fatal("failed catalog must not be cached")
	}
}

func TestServiceLogsInspectionWithoutObjectNames(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	s := NewServiceWithLogger(cache.NewMemCache(64), time.Minute, logger)
	intr := &fakeSchemaInspector{}
	ctx := context.Background()
	users := schemameta.ObjectRef{Namespace: "public", Kind: "table", Name: "users"}

	if _, err := s.Catalog(ctx, "c1", intr); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Objects(ctx, "c1", []schemameta.ObjectRef{users}, intr); err != nil {
		t.Fatal(err)
	}
	s.RefreshObject("c1", users)
	s.RefreshConnection("c1")

	logs := buf.String()
	for _, want := range []string{
		"schema catalog cache miss",
		"schema catalog inspected",
		"schema object detail cache checked",
		"schema object details inspected",
		"schema object cache invalidated",
		"schema connection cache invalidated",
		"conn_id=c1",
		"kind=table",
	} {
		if !strings.Contains(logs, want) {
			t.Fatalf("expected logs to contain %q, got:\n%s", want, logs)
		}
	}
	for _, sensitive := range []string{"users", "public"} {
		if strings.Contains(logs, sensitive) {
			t.Fatalf("logs should not contain object name/namespace %q:\n%s", sensitive, logs)
		}
	}
}
