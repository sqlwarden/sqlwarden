package schema

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/sqlwarden/internal/database"
	metadata "github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/platform/invalidate"
)

// droppingBus accepts subscriptions and publishes but never delivers, like a
// transport that loses every message.
type droppingBus struct {
	mu        sync.Mutex
	published []string
}

func (b *droppingBus) Publish(_ context.Context, topic invalidate.Topic, key string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.published = append(b.published, string(topic)+" "+key)
	return nil
}

func (b *droppingBus) Subscribe(invalidate.Topic, func(string)) func() { return func() {} }

type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func busNavigator(store Store, bus invalidate.Invalidator, clock *testClock) *Navigator {
	return NewNavigator(store, nil, WithInvalidator(bus), WithClock(clock.Now))
}

func databasesCatalog() *fakeCatalog {
	cat := newFakeCatalog()
	cat.set("", "databases", metadata.Child{Kind: "database", Name: "app"})
	cat.set(dbPath("app"), "schemas", metadata.Child{Kind: "schema", Name: "public"})
	return cat
}

func servedFromMemory(t *testing.T, n *Navigator, conn Connection, tree metadata.Tree) bool {
	t.Helper()
	listing, err := n.Children(context.Background(), conn, tree, nil, "", "databases")
	if errors.Is(err, ErrSessionRequired) {
		return false
	}
	if err != nil {
		t.Fatal(err)
	}
	return listing.Source == SourceMemory
}

func TestForeignSchemaInvalidationDropsConnectionMemory(t *testing.T) {
	ctx := context.Background()
	cat := databasesCatalog()
	tree := cat.Tree()
	bus := invalidate.NewMemory(nil)
	clock := &testClock{now: fixedNow()}
	publisher := busNavigator(nil, bus, clock)

	for name, invalidateConn := range map[string]func(Connection) error{
		"refresh": func(conn Connection) error {
			_, err := publisher.Refresh(ctx, conn, tree, cat, "")
			return err
		},
		"invalidate": func(conn Connection) error { return publisher.Invalidate(ctx, conn, "") },
		"purge": func(conn Connection) error {
			publisher.PurgeConnection(conn.ID)
			return nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			subscriber := busNavigator(nil, bus, clock)
			one, two := Connection{ID: 1}, Connection{ID: 2}
			for _, conn := range []Connection{one, two} {
				if _, err := subscriber.Children(ctx, conn, tree, cat, "", "databases"); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := publisher.Children(ctx, one, tree, cat, "", "databases"); err != nil {
				t.Fatal(err)
			}
			if err := invalidateConn(one); err != nil {
				t.Fatal(err)
			}
			if servedFromMemory(t, subscriber, one, tree) {
				t.Fatal("connection 1 memory survived a foreign invalidation")
			}
			if !servedFromMemory(t, subscriber, two, tree) {
				t.Fatal("connection 2 memory was dropped by an invalidation for connection 1")
			}
		})
	}
}

func TestOwnSchemaInvalidationIsIgnored(t *testing.T) {
	ctx := context.Background()
	cat := databasesCatalog()
	tree := cat.Tree()
	bus := invalidate.NewMemory(nil)
	n := busNavigator(nil, bus, &testClock{now: fixedNow()})
	conn := Connection{ID: 1}

	if _, err := n.Children(ctx, conn, tree, cat, "", "databases"); err != nil {
		t.Fatal(err)
	}
	if _, err := n.Refresh(ctx, conn, tree, cat, ""); err != nil {
		t.Fatal(err)
	}
	if !servedFromMemory(t, n, conn, tree) {
		t.Fatal("refresh wiped the memory it filled through its own publish")
	}
	if err := bus.Publish(ctx, invalidate.TopicSchema, invalidationKey(conn.ID, n.origin)); err != nil {
		t.Fatal(err)
	}
	if !servedFromMemory(t, n, conn, tree) {
		t.Fatal("own-origin key dropped memory")
	}
}

func TestNavigatorPublishesSchemaAndCompletionKeys(t *testing.T) {
	ctx := context.Background()
	cat := databasesCatalog()
	tree := cat.Tree()
	bus := &droppingBus{}
	n := busNavigator(nil, bus, &testClock{now: fixedNow()})
	conn := Connection{ID: 7}

	if _, err := n.Refresh(ctx, conn, tree, cat, ""); err != nil {
		t.Fatal(err)
	}
	if err := n.Invalidate(ctx, conn, ""); err != nil {
		t.Fatal(err)
	}
	n.PurgeConnection(conn.ID)
	key := "7/" + n.origin
	want := []string{"schema " + key, "completion " + key}
	want = slices.Concat(want, want, want)
	if !slices.Equal(bus.published, want) {
		t.Fatalf("published = %v, want %v", bus.published, want)
	}
}

func TestForgetConnectionDoesNotPublish(t *testing.T) {
	bus := &droppingBus{}
	n := busNavigator(nil, bus, &testClock{now: fixedNow()})

	n.ForgetConnection(7)

	if len(bus.published) != 0 {
		t.Fatalf("ForgetConnection published %v; it is local to this instance", bus.published)
	}
}

func TestNavigatorMemoryExpiresAtCeiling(t *testing.T) {
	ctx := context.Background()
	cat := databasesCatalog()
	users := metadata.ObjectRef{Scope: schemaPathOf("app", "public"), Kind: "table", Name: "users"}
	cat.objects[users] = metadata.Object{Ref: users}
	tree := cat.Tree()
	clock := &testClock{now: fixedNow()}
	n := busNavigator(nil, &droppingBus{}, clock)
	conn := Connection{ID: 1}

	if _, err := n.Children(ctx, conn, tree, cat, "", "databases"); err != nil {
		t.Fatal(err)
	}
	if _, err := n.Objects(ctx, conn, cat, []metadata.ObjectRef{users}); err != nil {
		t.Fatal(err)
	}
	clock.advance(navigatorMemoryTTL - time.Nanosecond)
	if !servedFromMemory(t, n, conn, tree) {
		t.Fatal("listing expired before the ceiling")
	}
	if _, err := n.Objects(ctx, conn, nil, []metadata.ObjectRef{users}); err != nil {
		t.Fatalf("object expired before the ceiling: %v", err)
	}

	clock.advance(time.Nanosecond)
	if servedFromMemory(t, n, conn, tree) {
		t.Fatal("listing served from memory at the ceiling")
	}
	listing, err := n.Children(ctx, conn, tree, cat, "", "databases")
	if err != nil {
		t.Fatal(err)
	}
	if listing.Source != SourceLive {
		t.Fatalf("expired listing source = %s, want live", listing.Source)
	}
	if _, err := n.Objects(ctx, conn, nil, []metadata.ObjectRef{users}); !errors.Is(err, ErrSessionRequired) {
		t.Fatalf("expired object err = %v, want session required", err)
	}
	if got := cat.callLog(); !slices.Equal(got, []string{"databases:1", "objects:1", "databases:1"}) {
		t.Fatalf("calls = %v", got)
	}
}

func TestDroppedSchemaInvalidationSelfHealsAtCeiling(t *testing.T) {
	ctx := context.Background()
	cat := databasesCatalog()
	tree := cat.Tree()
	bus := &droppingBus{}
	clock := &testClock{now: fixedNow()}
	publisher := busNavigator(nil, bus, clock)
	subscriber := busNavigator(nil, bus, clock)
	conn := Connection{ID: 1}

	if _, err := subscriber.Children(ctx, conn, tree, cat, "", "databases"); err != nil {
		t.Fatal(err)
	}
	cat.set("", "databases", metadata.Child{Kind: "database", Name: "app"}, metadata.Child{Kind: "database", Name: "added"})
	if _, err := publisher.Refresh(ctx, conn, tree, cat, ""); err != nil {
		t.Fatal(err)
	}
	if len(bus.published) == 0 {
		t.Fatal("refresh published nothing")
	}
	if !servedFromMemory(t, subscriber, conn, tree) {
		t.Fatal("a dropped message must leave the subscriber's memory in place")
	}
	clock.advance(navigatorMemoryTTL)
	listing, err := subscriber.Children(ctx, conn, tree, cat, "", "databases")
	if err != nil {
		t.Fatal(err)
	}
	if listing.Source != SourceLive || !slices.Equal(itemNames(listing), []string{"app", "added"}) {
		t.Fatalf("after ceiling = %+v", listing)
	}
}

func TestCompletionInvalidationClearsOnlyThatConnection(t *testing.T) {
	ctx := context.Background()
	cat := databasesCatalog()
	tree := cat.Tree()
	store := newFakeStore()
	bus := invalidate.NewMemory(nil)
	n := busNavigator(store, bus, &testClock{now: fixedNow()})
	live := &scopedCatalog{fakeCatalog: cat, scope: schemaPathOf("app", "public")}
	one, two := Connection{ID: 1, Persistent: true}, Connection{ID: 2, Persistent: true}
	for _, conn := range []Connection{one, two} {
		if _, err := n.Children(ctx, conn, tree, cat, "", "databases"); err != nil {
			t.Fatal(err)
		}
		if _, err := n.CompletionView(ctx, conn, tree, live); err != nil {
			t.Fatal(err)
		}
	}
	if n.decoded.previous(one.ID) == nil || n.decoded.previous(two.ID) == nil {
		t.Fatal("completion reads did not populate the decode memo")
	}

	if err := bus.Publish(ctx, invalidate.TopicCompletion, invalidationKey(one.ID, "peer")); err != nil {
		t.Fatal(err)
	}
	n.mu.Lock()
	_, scopeOne := n.sessionScopes[one.ID]
	_, scopeTwo := n.sessionScopes[two.ID]
	n.mu.Unlock()
	if scopeOne || !scopeTwo {
		t.Fatalf("session scopes after invalidation: one=%v two=%v", scopeOne, scopeTwo)
	}
	if n.decoded.previous(one.ID) != nil || n.decoded.previous(two.ID) == nil {
		t.Fatal("decode memo was not cleared for connection 1 only")
	}
	if !servedFromMemory(t, n, one, tree) {
		t.Fatal("a completion message must not drop navigator memory")
	}
}

func TestUnparseableInvalidationKeyClearsEveryConnection(t *testing.T) {
	ctx := context.Background()
	cat := databasesCatalog()
	tree := cat.Tree()
	store := newFakeStore()
	bus := invalidate.NewMemory(nil)
	n := busNavigator(store, bus, &testClock{now: fixedNow()})
	live := &scopedCatalog{fakeCatalog: cat, scope: schemaPathOf("app", "public")}
	conns := []Connection{{ID: 1}, {ID: 2}}
	for _, conn := range conns {
		if _, err := n.Children(ctx, conn, tree, cat, "", "databases"); err != nil {
			t.Fatal(err)
		}
		if _, err := n.CompletionView(ctx, conn, tree, live); err != nil {
			t.Fatal(err)
		}
	}

	for _, key := range []string{"garbage", "x/peer", "1/"} {
		if _, _, ok := parseInvalidationKey(key); ok {
			t.Fatalf("key %q parsed", key)
		}
	}
	if err := bus.Publish(ctx, invalidate.TopicCompletion, "garbage"); err != nil {
		t.Fatal(err)
	}
	n.mu.Lock()
	scopes := len(n.sessionScopes)
	n.mu.Unlock()
	if scopes != 0 || n.decoded.previous(1) != nil || n.decoded.previous(2) != nil {
		t.Fatal("unparseable completion key left completion caches in place")
	}
	for _, conn := range conns {
		if !servedFromMemory(t, n, conn, tree) {
			t.Fatal("a completion message must not drop navigator memory")
		}
	}

	if err := bus.Publish(ctx, invalidate.TopicSchema, "garbage"); err != nil {
		t.Fatal(err)
	}
	for _, conn := range conns {
		if servedFromMemory(t, n, conn, tree) {
			t.Fatalf("connection %d memory survived an unparseable schema key", conn.ID)
		}
	}
}

func TestDecodeMemoReusesValuesUntilCeiling(t *testing.T) {
	row, err := encodeListing(1, Listing{Parent: "", Folder: "databases", Items: []metadata.Child{{Kind: "database", Name: "app"}}})
	if err != nil {
		t.Fatal(err)
	}
	object := metadata.Object{Ref: metadata.ObjectRef{Scope: schemaPathOf("app", "public"), Kind: "table", Name: "users"}}
	data, err := gzipJSON(object)
	if err != nil {
		t.Fatal(err)
	}
	objectRow := database.SchemaObject{ConnectionID: 1, Scope: string(object.Ref.Scope), Kind: object.Ref.Kind, Name: object.Ref.Name, ObjectData: data}
	listingKeyOf := listingKey{parent: "", folder: "databases"}
	objectKey := schemaObjectKey(object.Ref)

	memo := newDecodeMemo()
	start := fixedNow()
	pass := func(at time.Time) *memoRead {
		t.Helper()
		read := memo.begin(1, at)
		if _, err := read.listing(row); err != nil {
			t.Fatal(err)
		}
		if _, err := read.object(objectRow); err != nil {
			t.Fatal(err)
		}
		read.commit()
		return read
	}

	pass(start)
	reused := pass(start.Add(decodeMemoTTL - time.Nanosecond))
	if !reused.next.listings[listingKeyOf].at.Equal(start) || !reused.next.objects[objectKey].at.Equal(start) {
		t.Fatal("unchanged blobs were decoded again before the ceiling")
	}
	expired := start.Add(decodeMemoTTL)
	redecoded := pass(expired)
	if !redecoded.next.listings[listingKeyOf].at.Equal(expired) || !redecoded.next.objects[objectKey].at.Equal(expired) {
		t.Fatal("decoded values were reused at the ceiling")
	}
}
