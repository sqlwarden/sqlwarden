package schema

import (
	"context"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/sqlwarden/internal/database"
	metadata "github.com/sqlwarden/internal/engine/metadata"
)

func seg(kind, name string) metadata.ScopeSegment {
	return metadata.ScopeSegment{Kind: kind, Name: name}
}

func dbPath(db string) metadata.ScopePath { return metadata.NewScopePath(seg("database", db)) }

func schemaPathOf(db, schema string) metadata.ScopePath {
	return dbPath(db).Child(seg("schema", schema))
}

func tablePathOf(db, schema, table string) metadata.ScopePath {
	return schemaPathOf(db, schema).Child(seg("table", table))
}

// fakeCatalog is a SchemaInspector whose loaders read an in-memory catalog
// and record every call as "folder:parentCount".
type fakeCatalog struct {
	mu        sync.Mutex
	children  map[string][]metadata.Child
	objects   map[metadata.ObjectRef]metadata.Object
	calls     []string
	databases []string
	fail      error
	// entered and release, when set, make every loader signal entry and
	// block until release is closed.
	entered chan struct{}
	release chan struct{}
}

func newFakeCatalog() *fakeCatalog {
	return &fakeCatalog{children: map[string][]metadata.Child{}, objects: map[metadata.ObjectRef]metadata.Object{}}
}

func (c *fakeCatalog) set(parent metadata.ScopePath, folder string, kids ...metadata.Child) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.children[string(parent)+"#"+folder] = kids
}

func (c *fakeCatalog) callLog() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.calls...)
}

func (c *fakeCatalog) loader(folder string) metadata.Loader {
	return func(_ context.Context, _ metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
		if c.release != nil {
			c.entered <- struct{}{}
			<-c.release
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.fail != nil {
			return nil, c.fail
		}
		c.calls = append(c.calls, folder+":"+strconv.Itoa(len(parents)))
		out := map[metadata.ScopePath][]metadata.Child{}
		for _, p := range parents {
			if kids, ok := c.children[string(p)+"#"+folder]; ok {
				out[p] = kids
			}
		}
		return out, nil
	}
}

func (c *fakeCatalog) Tree() metadata.Tree {
	return metadata.Tree{
		SystemObjects: true,
		Root: metadata.Node{Label: "Connection", Icon: "connection", Folders: []metadata.Folder{
			{Kind: "databases", Label: "Databases", Child: "database", List: c.loader("databases")},
		}},
		Nodes: map[string]metadata.Node{
			"database": {Label: "Database", Icon: "database", Scope: true, ShowAllDatabases: true, Folders: []metadata.Folder{
				{Kind: "schemas", Label: "Schemas", Child: "schema", List: c.loader("schemas")},
			}},
			"schema": {Label: "Schema", Icon: "schema", Scope: true, Folders: []metadata.Folder{
				{Kind: "tables", Label: "Tables", Child: "table", List: c.loader("tables")},
				{Kind: "functions", Label: "Functions", Child: "function", MixedKinds: []string{"procedure"}, List: c.loader("functions")},
			}},
			"table": {Label: "Table", Icon: "table", Relational: true, Folders: []metadata.Folder{
				{Kind: "columns", Label: "Columns", Child: "column", List: c.loader("columns")},
			}},
			"column":    {Label: "Column", Icon: "column", Leaf: true, Column: true},
			"function":  {Label: "Function", Icon: "function", Leaf: true},
			"procedure": {Label: "Procedure", Icon: "procedure", Leaf: true},
		},
	}
}

func (c *fakeCatalog) Querier(_ context.Context, database string) (metadata.Querier, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.databases = append(c.databases, database)
	return nil, nil
}

func (c *fakeCatalog) InspectObjects(_ context.Context, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, "objects:"+strconv.Itoa(len(refs)))
	var out []metadata.Object
	for _, ref := range refs {
		if obj, ok := c.objects[ref]; ok {
			out = append(out, obj)
		}
	}
	return out, nil
}

// fakeStore mirrors database.DB's schema cache semantics in memory.
type fakeStore struct {
	mu            sync.Mutex
	listings      map[string]database.SchemaListing
	objects       map[database.SchemaObjectKey]database.SchemaObject
	relationships map[string]database.SchemaRelationship
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		listings:      map[string]database.SchemaListing{},
		objects:       map[database.SchemaObjectKey]database.SchemaObject{},
		relationships: map[string]database.SchemaRelationship{},
	}
}

func within(path, root string) bool {
	return metadata.ScopePath(path).Within(metadata.ScopePath(root))
}

func (s *fakeStore) SchemaListing(_ context.Context, _ int64, parentPath, folder string) (database.SchemaListing, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.listings[parentPath+"#"+folder]
	return row, ok, nil
}

func (s *fakeStore) SchemaListingsWithin(_ context.Context, _ int64, root string) ([]database.SchemaListing, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []database.SchemaListing
	for _, row := range s.listings {
		if within(row.ParentPath, root) {
			out = append(out, row)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ParentPath+out[i].Folder < out[j].ParentPath+out[j].Folder })
	return out, nil
}

func (s *fakeStore) UpsertSchemaListings(_ context.Context, rows []database.SchemaListing) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, row := range rows {
		s.listings[row.ParentPath+"#"+row.Folder] = row
	}
	return nil
}

func (s *fakeStore) DeleteSchemaListingsWithin(_ context.Context, _ int64, root string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, row := range s.listings {
		if within(row.ParentPath, root) {
			delete(s.listings, key)
		}
	}
	return nil
}

func (s *fakeStore) SchemaObjects(_ context.Context, _ int64, keys []database.SchemaObjectKey) ([]database.SchemaObject, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []database.SchemaObject
	for _, key := range keys {
		if row, ok := s.objects[key]; ok {
			out = append(out, row)
		}
	}
	return out, nil
}

func (s *fakeStore) AllSchemaObjects(context.Context, int64) ([]database.SchemaObject, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []database.SchemaObject
	for _, row := range s.objects {
		out = append(out, row)
	}
	return out, nil
}

func (s *fakeStore) UpsertSchemaObjects(_ context.Context, rows []database.SchemaObject) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, row := range rows {
		s.objects[database.SchemaObjectKey{Scope: row.Scope, Kind: row.Kind, Name: row.Name}] = row
	}
	return nil
}

func (s *fakeStore) DeleteSchemaObjects(_ context.Context, _ int64, root string, exact []database.SchemaObjectKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key := range s.objects {
		if within(key.Scope, root) {
			delete(s.objects, key)
		}
	}
	for _, key := range exact {
		delete(s.objects, key)
	}
	return nil
}

func (s *fakeStore) SchemaRelationship(_ context.Context, _ int64, scope string) (database.SchemaRelationship, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.relationships[scope]
	return row, ok, nil
}

func (s *fakeStore) UpsertSchemaRelationship(_ context.Context, row database.SchemaRelationship) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.relationships[row.Scope] = row
	return nil
}

func (s *fakeStore) DeleteSchemaRelationships(_ context.Context, _ int64, root string, exact []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for scope := range s.relationships {
		if within(scope, root) {
			delete(s.relationships, scope)
		}
	}
	for _, scope := range exact {
		delete(s.relationships, scope)
	}
	return nil
}

func fixedNow() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) }

func newTestNavigator(store Store) *Navigator {
	n := NewNavigator(store, nil)
	n.now = fixedNow
	return n
}
