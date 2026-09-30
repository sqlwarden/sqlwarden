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
}

// navigatorLoadTimeout bounds a shared live load so a hung target cannot pin
// its singleflight key and block every later request for the same folder.
const navigatorLoadTimeout = 2 * time.Minute

func NewNavigator(store Store, logger *slog.Logger) *Navigator {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Navigator{store: store, logger: logger, now: time.Now, memory: map[int64]*connectionMemory{}}
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
}

func (n *Navigator) Children(ctx context.Context, conn Connection, tree metadata.Tree, live metadata.SchemaInspector, parent metadata.ScopePath, folderKind string) (Listing, error) {
	folder, ok := tree.Folder(tree.NodeKindOf(parent), folderKind)
	if !ok {
		return Listing{}, ErrUnknownFolder
	}
	listing, err := n.cachedOrLoad(ctx, conn, tree, live, parent, folder)
	if err != nil {
		return Listing{}, err
	}
	return n.present(conn, tree, listing), nil
}

func (n *Navigator) cachedOrLoad(ctx context.Context, conn Connection, tree metadata.Tree, live metadata.SchemaInspector, parent metadata.ScopePath, folder metadata.Folder) (Listing, error) {
	key := listingKey{parent: parent, folder: folder.Kind}
	n.mu.Lock()
	cached, ok := n.memoryFor(conn.ID).listings[key]
	n.mu.Unlock()
	if ok {
		cached.Source = SourceMemory
		return cached, nil
	}
	if conn.Persistent && n.store != nil {
		row, found, err := n.store.SchemaListing(ctx, conn.ID, string(parent), folder.Kind)
		if err != nil {
			return Listing{}, fmt.Errorf("read cached listing: %w", err)
		}
		if found {
			listing, err := decodeListing(row)
			if err != nil {
				return Listing{}, err
			}
			n.remember(conn.ID, listing)
			listing.Source = SourceStore
			return listing, nil
		}
	}
	if live == nil {
		return Listing{}, ErrSessionRequired
	}
	// The shared load outlives any single waiter's cancellation so coalesced
	// callers are not failed by whichever request happened to start it.
	flightKey := fmt.Sprintf("%d\x00%s\x00%s", conn.ID, parent, folder.Kind)
	result := n.group.DoChan(flightKey, func() (any, error) {
		flightCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), navigatorLoadTimeout)
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
	select {
	case <-ctx.Done():
		return Listing{}, ctx.Err()
	case res := <-result:
		if res.Err != nil {
			return Listing{}, res.Err
		}
		listing := res.Val.(Listing)
		listing.Source = SourceLive
		return listing, nil
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
