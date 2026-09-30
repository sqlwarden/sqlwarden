package schema

import (
	"context"
	"errors"
	"slices"
	"testing"

	metadata "github.com/sqlwarden/internal/engine/metadata"
)

func itemNames(l Listing) []string {
	out := make([]string, 0, len(l.Items))
	for _, item := range l.Items {
		out = append(out, item.Name)
	}
	return out
}

func TestChildrenLoadsLiveThenServesMemory(t *testing.T) {
	cat := newFakeCatalog()
	cat.set("", "databases", metadata.Child{Kind: "database", Name: "app", Current: true}, metadata.Child{Kind: "database", Name: "other"})
	n := newTestNavigator(nil)
	conn := Connection{ID: 1}
	ctx := context.Background()

	first, err := n.Children(ctx, conn, cat.Tree(), cat, "", "databases")
	if err != nil {
		t.Fatal(err)
	}
	if first.Source != SourceLive || !slices.Equal(itemNames(first), []string{"app", "other"}) || !first.FetchedAt.Equal(fixedNow()) {
		t.Fatalf("first = %+v", first)
	}
	second, err := n.Children(ctx, conn, cat.Tree(), nil, "", "databases")
	if err != nil {
		t.Fatal(err)
	}
	if second.Source != SourceMemory {
		t.Fatalf("second source = %s", second.Source)
	}
	if got := cat.callLog(); !slices.Equal(got, []string{"databases:1"}) {
		t.Fatalf("calls = %v", got)
	}
}

func TestChildrenRoutesQuerierByDatabase(t *testing.T) {
	cat := newFakeCatalog()
	cat.set(dbPath("sales"), "schemas", metadata.Child{Kind: "schema", Name: "public"})
	n := newTestNavigator(nil)
	if _, err := n.Children(context.Background(), Connection{ID: 1}, cat.Tree(), cat, dbPath("sales"), "schemas"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(cat.databases, []string{"sales"}) {
		t.Fatalf("querier databases = %v", cat.databases)
	}
}

func TestChildrenWithoutSessionRequiresSession(t *testing.T) {
	cat := newFakeCatalog()
	n := newTestNavigator(newFakeStore())
	_, err := n.Children(context.Background(), Connection{ID: 1, Persistent: true}, cat.Tree(), nil, "", "databases")
	if !errors.Is(err, ErrSessionRequired) {
		t.Fatalf("err = %v", err)
	}
	if len(cat.callLog()) != 0 {
		t.Fatal("no loader may run without a session")
	}
}

func TestChildrenUnknownFolder(t *testing.T) {
	cat := newFakeCatalog()
	n := newTestNavigator(nil)
	_, err := n.Children(context.Background(), Connection{ID: 1}, cat.Tree(), cat, dbPath("app"), "tables")
	if !errors.Is(err, ErrUnknownFolder) {
		t.Fatalf("err = %v", err)
	}
}

func TestChildrenPersistsAndWarmsFromStore(t *testing.T) {
	cat := newFakeCatalog()
	cat.set(dbPath("app"), "schemas", metadata.Child{Kind: "schema", Name: "public", Attributes: map[string]any{"owner": "app"}})
	store := newFakeStore()
	conn := Connection{ID: 7, Persistent: true}
	ctx := context.Background()

	if _, err := newTestNavigator(store).Children(ctx, conn, cat.Tree(), cat, dbPath("app"), "schemas"); err != nil {
		t.Fatal(err)
	}
	restarted := newTestNavigator(store)
	fromStore, err := restarted.Children(ctx, conn, cat.Tree(), nil, dbPath("app"), "schemas")
	if err != nil {
		t.Fatal(err)
	}
	if fromStore.Source != SourceStore || fromStore.Items[0].Attributes["owner"] != "app" {
		t.Fatalf("fromStore = %+v", fromStore)
	}
	again, _ := restarted.Children(ctx, conn, cat.Tree(), nil, dbPath("app"), "schemas")
	if again.Source != SourceMemory {
		t.Fatalf("store hit must warm memory, got %s", again.Source)
	}
}

func TestChildrenEphemeralModeSkipsStore(t *testing.T) {
	cat := newFakeCatalog()
	cat.set("", "databases", metadata.Child{Kind: "database", Name: "app"})
	store := newFakeStore()
	if _, err := newTestNavigator(store).Children(context.Background(), Connection{ID: 1}, cat.Tree(), cat, "", "databases"); err != nil {
		t.Fatal(err)
	}
	if len(store.listings) != 0 {
		t.Fatal("non-persistent connections must not write the store")
	}
}

func TestPresentHidesSystemObjects(t *testing.T) {
	cat := newFakeCatalog()
	cat.set(dbPath("app"), "schemas", metadata.Child{Kind: "schema", Name: "pg_catalog", System: true}, metadata.Child{Kind: "schema", Name: "public"})
	n := newTestNavigator(nil)
	ctx := context.Background()
	hidden, _ := n.Children(ctx, Connection{ID: 1}, cat.Tree(), cat, dbPath("app"), "schemas")
	if !slices.Equal(itemNames(hidden), []string{"public"}) {
		t.Fatalf("hidden = %v", itemNames(hidden))
	}
	shown, _ := n.Children(ctx, Connection{ID: 1, ShowSystem: true}, cat.Tree(), nil, dbPath("app"), "schemas")
	if !slices.Equal(itemNames(shown), []string{"pg_catalog", "public"}) {
		t.Fatalf("toggling show-system must not refetch: %v", itemNames(shown))
	}
}

func TestPresentShowAllDatabasesMatrix(t *testing.T) {
	cat := newFakeCatalog()
	cat.set("", "databases",
		metadata.Child{Kind: "database", Name: "app", Current: true},
		metadata.Child{Kind: "database", Name: "reports"},
	)
	n := newTestNavigator(nil)
	ctx := context.Background()
	tree := cat.Tree()
	if _, err := n.Children(ctx, Connection{ID: 1}, tree, cat, "", "databases"); err != nil {
		t.Fatal(err)
	}
	current := func(l Listing) []string {
		var out []string
		for _, item := range l.Items {
			if item.Current {
				out = append(out, item.Name)
			}
		}
		return out
	}

	noDefault, _ := n.Children(ctx, Connection{ID: 1}, tree, nil, "", "databases")
	if !slices.Equal(itemNames(noDefault), []string{"app", "reports"}) || !slices.Equal(current(noDefault), []string{"app"}) {
		t.Fatalf("no default = %v current %v", itemNames(noDefault), current(noDefault))
	}
	showAll, _ := n.Children(ctx, Connection{ID: 1, DefaultScope: dbPath("reports"), ShowAllDatabases: true}, tree, nil, "", "databases")
	if !slices.Equal(itemNames(showAll), []string{"app", "reports"}) || !slices.Equal(current(showAll), []string{"reports"}) {
		t.Fatalf("show all = %v current %v", itemNames(showAll), current(showAll))
	}
	onlyDefault, _ := n.Children(ctx, Connection{ID: 1, DefaultScope: dbPath("reports")}, tree, nil, "", "databases")
	if !slices.Equal(itemNames(onlyDefault), []string{"reports"}) || !slices.Equal(current(onlyDefault), []string{"reports"}) {
		t.Fatalf("only default = %v current %v", itemNames(onlyDefault), current(onlyDefault))
	}
}

func TestPresentSynthesizesMissingDefaultDatabase(t *testing.T) {
	cat := newFakeCatalog()
	cat.set("", "databases", metadata.Child{Kind: "database", Name: "app"})
	n := newTestNavigator(nil)
	ctx := context.Background()
	conn := Connection{ID: 1, DefaultScope: dbPath("restricted"), ShowAllDatabases: true}
	got, err := n.Children(ctx, conn, cat.Tree(), cat, "", "databases")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(itemNames(got), []string{"restricted", "app"}) || !got.Items[0].Current || got.Items[0].Kind != "database" {
		t.Fatalf("got = %+v", got.Items)
	}
	raw, _ := n.Children(ctx, Connection{ID: 1}, cat.Tree(), nil, "", "databases")
	if !slices.Equal(itemNames(raw), []string{"app"}) {
		t.Fatalf("synthesized database must not be cached: %v", itemNames(raw))
	}
}

func TestForgetConnectionClearsMemory(t *testing.T) {
	cat := newFakeCatalog()
	cat.set("", "databases", metadata.Child{Kind: "database", Name: "app"})
	n := newTestNavigator(nil)
	ctx := context.Background()
	_, _ = n.Children(ctx, Connection{ID: 1}, cat.Tree(), cat, "", "databases")
	n.ForgetConnection(1)
	if _, err := n.Children(ctx, Connection{ID: 1}, cat.Tree(), nil, "", "databases"); !errors.Is(err, ErrSessionRequired) {
		t.Fatalf("err = %v", err)
	}
}

func TestChildrenCanceledWaiterDoesNotFailSharedLoad(t *testing.T) {
	cat := newFakeCatalog()
	cat.set("", "databases", metadata.Child{Kind: "database", Name: "app"})
	cat.entered = make(chan struct{}, 1)
	cat.release = make(chan struct{})
	n := newTestNavigator(nil)
	conn := Connection{ID: 1}

	leaderCtx, cancel := context.WithCancel(context.Background())
	leaderErr := make(chan error, 1)
	go func() {
		_, err := n.Children(leaderCtx, conn, cat.Tree(), cat, "", "databases")
		leaderErr <- err
	}()
	<-cat.entered
	cancel()
	if err := <-leaderErr; !errors.Is(err, context.Canceled) {
		t.Fatalf("leader err = %v, want context.Canceled", err)
	}

	followerDone := make(chan Listing, 1)
	go func() {
		listing, err := n.Children(context.Background(), conn, cat.Tree(), cat, "", "databases")
		if err != nil {
			t.Error(err)
		}
		followerDone <- listing
	}()
	close(cat.release)
	if got := itemNames(<-followerDone); !slices.Equal(got, []string{"app"}) {
		t.Fatalf("follower items = %v", got)
	}
	if got := cat.callLog(); !slices.Equal(got, []string{"databases:1"}) {
		t.Fatalf("calls = %v", got)
	}
}
