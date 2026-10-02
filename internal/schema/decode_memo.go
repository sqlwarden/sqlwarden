package schema

import (
	"hash/maphash"
	"sync"

	"github.com/sqlwarden/internal/database"
	metadata "github.com/sqlwarden/internal/engine/metadata"
)

var memoSeed = maphash.MakeSeed()

type blobID struct {
	sum  uint64
	size int
}

func identify(data []byte) blobID {
	return blobID{sum: maphash.Bytes(memoSeed, data), size: len(data)}
}

type memoListing struct {
	id      blobID
	listing Listing
}

type memoObject struct {
	id     blobID
	object metadata.Object
}

type connectionMemo struct {
	listings map[listingKey]memoListing
	objects  map[database.SchemaObjectKey]memoObject
}

// decodeMemo remembers the decoded form of stored listing and object blobs,
// keyed by the blob's content, so repeated completion reads decode only rows
// that changed. A row whose blob differs is decoded again, and rows no longer
// present are dropped when a read commits. Decoded values are shared between
// reads and must be treated as read-only.
type decodeMemo struct {
	mu    sync.Mutex
	conns map[int64]*connectionMemo
}

func newDecodeMemo() *decodeMemo { return &decodeMemo{conns: map[int64]*connectionMemo{}} }

func (m *decodeMemo) forget(connID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.conns, connID)
}

func (m *decodeMemo) previous(connID int64) *connectionMemo {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.conns[connID]
}

// memoRead is one read pass over a connection's stored rows. A nil *memoRead
// decodes every row without remembering anything.
type memoRead struct {
	memo   *decodeMemo
	connID int64
	prev   *connectionMemo
	next   *connectionMemo
}

func (m *decodeMemo) begin(connID int64) *memoRead {
	return &memoRead{
		memo:   m,
		connID: connID,
		prev:   m.previous(connID),
		next:   &connectionMemo{listings: map[listingKey]memoListing{}, objects: map[database.SchemaObjectKey]memoObject{}},
	}
}

func (r *memoRead) commit() {
	if r == nil {
		return
	}
	r.memo.mu.Lock()
	defer r.memo.mu.Unlock()
	r.memo.conns[r.connID] = r.next
}

func (r *memoRead) listing(row database.SchemaListing) (Listing, error) {
	if r == nil {
		return decodeListing(row)
	}
	key := listingKey{parent: metadata.ScopePath(row.ParentPath), folder: row.Folder}
	id := identify(row.ChildrenData)
	if r.prev != nil {
		if hit, ok := r.prev.listings[key]; ok && hit.id == id {
			r.next.listings[key] = hit
			return hit.listing, nil
		}
	}
	listing, err := decodeListing(row)
	if err != nil {
		return Listing{}, err
	}
	r.next.listings[key] = memoListing{id: id, listing: listing}
	return listing, nil
}

func (r *memoRead) object(row database.SchemaObject) (metadata.Object, error) {
	if r == nil {
		var obj metadata.Object
		err := gunzipJSON(row.ObjectData, &obj)
		return obj, err
	}
	key := database.SchemaObjectKey{Scope: row.Scope, Kind: row.Kind, Name: row.Name}
	id := identify(row.ObjectData)
	if r.prev != nil {
		if hit, ok := r.prev.objects[key]; ok && hit.id == id {
			r.next.objects[key] = hit
			return hit.object, nil
		}
	}
	var obj metadata.Object
	if err := gunzipJSON(row.ObjectData, &obj); err != nil {
		return metadata.Object{}, err
	}
	r.next.objects[key] = memoObject{id: id, object: obj}
	return obj, nil
}
