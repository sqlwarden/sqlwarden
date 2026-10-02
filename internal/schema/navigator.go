package schema

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/sqlwarden/internal/database"
	metadata "github.com/sqlwarden/internal/engine/metadata"
	"golang.org/x/sync/singleflight"
)

var (
	ErrSessionRequired = errors.New("schema: live session required")
	ErrUnknownFolder   = errors.New("schema: unknown folder")
)

const (
	SourceMemory = "memory"
	SourceStore  = "store"
	SourceLive   = "live"
)

// Connection carries the per-connection settings the navigator applies at
// read time. Persistent gates the store.
type Connection struct {
	ID               int64
	DefaultScope     metadata.ScopePath
	ShowSystem       bool
	ShowAllDatabases bool
	Persistent       bool
}

type Listing struct {
	Parent    metadata.ScopePath
	Folder    string
	Items     []metadata.Child
	FetchedAt time.Time
	Source    string
}

type Store interface {
	SchemaListing(ctx context.Context, connID int64, parentPath, folder string) (database.SchemaListing, bool, error)
	SchemaListingsWithin(ctx context.Context, connID int64, root string) ([]database.SchemaListing, error)
	UpsertSchemaListings(ctx context.Context, rows []database.SchemaListing) error
	DeleteSchemaListingsWithin(ctx context.Context, connID int64, root string) error
	SchemaObjects(ctx context.Context, connID int64, keys []database.SchemaObjectKey) ([]database.SchemaObject, error)
	AllSchemaObjects(ctx context.Context, connID int64) ([]database.SchemaObject, error)
	UpsertSchemaObjects(ctx context.Context, rows []database.SchemaObject) error
	DeleteSchemaObjects(ctx context.Context, connID int64, within string, exact []database.SchemaObjectKey) error
	SchemaRelationship(ctx context.Context, connID int64, scope string) (database.SchemaRelationship, bool, error)
	UpsertSchemaRelationship(ctx context.Context, row database.SchemaRelationship) error
	DeleteSchemaRelationships(ctx context.Context, connID int64, within string, exact []string) error
}

type listingKey struct {
	parent metadata.ScopePath
	folder string
}

type connectionMemory struct {
	listings      map[listingKey]Listing
	objects       map[metadata.ObjectRef]metadata.Object
	relationships map[metadata.ScopePath]*metadata.RelationshipGraph
}

// Navigator serves lazily loaded navigator listings from memory, then the
// persistent store, then the caller's live session. It never opens a
// connection itself.
type Navigator struct {
	store  Store
	logger *slog.Logger
	now    func() time.Time
	group  singleflight.Group

	mu     sync.Mutex
	memory map[int64]*connectionMemory
	// inflight holds the flight keys of live loads whose singleflight call
	// has not been forgotten yet. It is only read or written, and group.DoChan
	// and group.Forget are only called, while holding mu, so presence here
	// exactly means a new caller for the key would join an existing load.
	inflight map[string]struct{}
	// sessionScopes memoizes each connection's live session scope.
	sessionScopes map[int64]sessionScopeMemo

	decoded *decodeMemo
}

// navigatorLoadTimeout bounds a shared live load so a hung target cannot pin
// its singleflight key and block every later request for the same folder.
const navigatorLoadTimeout = 2 * time.Minute

func NewNavigator(store Store, logger *slog.Logger) *Navigator {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Navigator{store: store, logger: logger, now: time.Now, memory: map[int64]*connectionMemory{}, inflight: map[string]struct{}{}, sessionScopes: map[int64]sessionScopeMemo{}, decoded: newDecodeMemo()}
}

// memoryFor returns the connection's memory cache; callers must hold n.mu.
func (n *Navigator) memoryFor(connID int64) *connectionMemory {
	mem, ok := n.memory[connID]
	if !ok {
		mem = &connectionMemory{
			listings:      map[listingKey]Listing{},
			objects:       map[metadata.ObjectRef]metadata.Object{},
			relationships: map[metadata.ScopePath]*metadata.RelationshipGraph{},
		}
		n.memory[connID] = mem
	}
	return mem
}

func (n *Navigator) ForgetConnection(connID int64) {
	n.mu.Lock()
	defer n.mu.Unlock()
	delete(n.memory, connID)
	delete(n.sessionScopes, connID)
	n.decoded.forget(connID)
}

func (n *Navigator) Children(ctx context.Context, conn Connection, tree metadata.Tree, live metadata.SchemaInspector, parent metadata.ScopePath, folderKind string) (Listing, error) {
	folder, ok := tree.Folder(tree.NodeKindOf(parent), folderKind)
	if !ok {
		return Listing{}, ErrUnknownFolder
	}
	listing, _, err := n.cachedOrLoad(ctx, conn, tree, live, parent, folder, navigatorLoadTimeout)
	if err != nil {
		return Listing{}, err
	}
	return n.present(conn, tree, listing), nil
}

// loadOrigin reports how cachedOrLoad obtained a listing: from the memory or
// store cache, by starting a live load, or by joining one already running.
type loadOrigin int

const (
	originNone loadOrigin = iota
	originCache
	originStarted
	originJoined
)

func flightKeyFor(connID int64, parent metadata.ScopePath, folder string) string {
	return fmt.Sprintf("%d\x00%s\x00%s", connID, parent, folder)
}

// cachedOrLoad serves the listing from memory, then the store, then a shared
// live load bounded by timeout. No live load starts once ctx is done.
func (n *Navigator) cachedOrLoad(ctx context.Context, conn Connection, tree metadata.Tree, live metadata.SchemaInspector, parent metadata.ScopePath, folder metadata.Folder, timeout time.Duration) (Listing, loadOrigin, error) {
	key := listingKey{parent: parent, folder: folder.Kind}
	n.mu.Lock()
	cached, ok := n.memoryFor(conn.ID).listings[key]
	n.mu.Unlock()
	if ok {
		cached.Source = SourceMemory
		return cached, originCache, nil
	}
	if conn.Persistent && n.store != nil {
		row, found, err := n.store.SchemaListing(ctx, conn.ID, string(parent), folder.Kind)
		if err != nil {
			return Listing{}, originNone, fmt.Errorf("read cached listing: %w", err)
		}
		if found {
			listing, err := decodeListing(row)
			if err != nil {
				return Listing{}, originNone, err
			}
			n.remember(conn.ID, listing)
			listing.Source = SourceStore
			return listing, originCache, nil
		}
	}
	if live == nil {
		return Listing{}, originNone, ErrSessionRequired
	}
	flightKey := flightKeyFor(conn.ID, parent, folder.Kind)
	n.mu.Lock()
	// Re-checked under mu: a load that finished since the first check has
	// already remembered its listing and forgotten its flight.
	if cached, ok := n.memoryFor(conn.ID).listings[key]; ok {
		n.mu.Unlock()
		cached.Source = SourceMemory
		return cached, originCache, nil
	}
	if err := ctx.Err(); err != nil {
		n.mu.Unlock()
		return Listing{}, originNone, err
	}
	origin := originStarted
	if _, running := n.inflight[flightKey]; running {
		origin = originJoined
	} else {
		n.inflight[flightKey] = struct{}{}
	}
	// The shared load outlives any single waiter's cancellation so coalesced
	// callers are not failed by whichever request happened to start it.
	result := n.group.DoChan(flightKey, func() (any, error) {
		defer func() {
			n.mu.Lock()
			n.group.Forget(flightKey)
			delete(n.inflight, flightKey)
			n.mu.Unlock()
		}()
		flightCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
		defer cancel()
		listings, err := n.load(flightCtx, live, tree.DatabaseOf(parent), folder, []metadata.ScopePath{parent})
		if err != nil {
			return nil, err
		}
		if err := n.save(flightCtx, conn, listings); err != nil {
			return nil, err
		}
		return listings[0], nil
	})
	n.mu.Unlock()
	select {
	case <-ctx.Done():
		return Listing{}, origin, ctx.Err()
	case res := <-result:
		if res.Err != nil {
			return Listing{}, origin, res.Err
		}
		listing := res.Val.(Listing)
		listing.Source = SourceLive
		return listing, origin, nil
	}
}

// load runs folder's loader once for all parents, which must share database.
// Every parent gets a listing, empty when the loader returned nothing for it.
func (n *Navigator) load(ctx context.Context, live metadata.SchemaInspector, database string, folder metadata.Folder, parents []metadata.ScopePath) ([]Listing, error) {
	q, err := live.Querier(ctx, database)
	if err != nil {
		return nil, err
	}
	children, err := folder.List(ctx, q, parents)
	if err != nil {
		return nil, err
	}
	fetchedAt := n.now().UTC()
	out := make([]Listing, 0, len(parents))
	for _, parent := range parents {
		items := children[parent]
		if items == nil {
			items = []metadata.Child{}
		}
		out = append(out, Listing{Parent: parent, Folder: folder.Kind, Items: items, FetchedAt: fetchedAt})
	}
	return out, nil
}

func (n *Navigator) remember(connID int64, listings ...Listing) {
	n.mu.Lock()
	defer n.mu.Unlock()
	mem := n.memoryFor(connID)
	for _, listing := range listings {
		listing.Source = ""
		mem.listings[listingKey{parent: listing.Parent, folder: listing.Folder}] = listing
	}
}

func (n *Navigator) save(ctx context.Context, conn Connection, listings []Listing) error {
	n.remember(conn.ID, listings...)
	if !conn.Persistent || n.store == nil || len(listings) == 0 {
		return nil
	}
	rows := make([]database.SchemaListing, 0, len(listings))
	for _, listing := range listings {
		row, err := encodeListing(conn.ID, listing)
		if err != nil {
			return err
		}
		rows = append(rows, row)
	}
	if err := n.store.UpsertSchemaListings(ctx, rows); err != nil {
		return fmt.Errorf("store listings: %w", err)
	}
	return nil
}

// present applies read-time connection settings: system-object hiding, the
// show-all-databases filter, and marking the default database current. It
// never mutates cached data.
func (n *Navigator) present(conn Connection, tree metadata.Tree, listing Listing) Listing {
	items := make([]metadata.Child, 0, len(listing.Items))
	for _, item := range listing.Items {
		if item.System && !conn.ShowSystem {
			continue
		}
		items = append(items, item)
	}
	databaseKind := tree.DatabaseKind()
	folder, _ := tree.Folder(tree.NodeKindOf(listing.Parent), listing.Folder)
	defaultDatabase := ""
	if databaseKind != "" {
		defaultDatabase = conn.DefaultScope.Name(databaseKind)
	}
	if databaseKind != "" && folder.Child == databaseKind && defaultDatabase != "" {
		found := false
		filtered := items[:0]
		for _, item := range items {
			item.Current = item.Name == defaultDatabase
			found = found || item.Current
			if conn.ShowAllDatabases || item.Current {
				filtered = append(filtered, item)
			}
		}
		items = filtered
		if !found {
			items = append([]metadata.Child{{Kind: databaseKind, Name: defaultDatabase, Current: true}}, items...)
		}
	}
	listing.Items = items
	return listing
}

func encodeListing(connID int64, listing Listing) (database.SchemaListing, error) {
	data, err := gzipJSON(listing.Items)
	if err != nil {
		return database.SchemaListing{}, fmt.Errorf("encode listing: %w", err)
	}
	return database.SchemaListing{
		ConnectionID: connID,
		ParentPath:   string(listing.Parent),
		Folder:       listing.Folder,
		ChildrenData: data,
		FetchedAt:    listing.FetchedAt,
	}, nil
}

func decodeListing(row database.SchemaListing) (Listing, error) {
	var items []metadata.Child
	if err := gunzipJSON(row.ChildrenData, &items); err != nil {
		return Listing{}, fmt.Errorf("decode listing: %w", err)
	}
	if items == nil {
		items = []metadata.Child{}
	}
	return Listing{Parent: metadata.ScopePath(row.ParentPath), Folder: row.Folder, Items: items, FetchedAt: row.FetchedAt.UTC()}, nil
}
