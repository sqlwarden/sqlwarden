package schema

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	catalogPrefix = "cat:"
	objectPrefix  = "obj:"
	sep           = "\x00"
)

func catalogKey(connID string) string { return catalogPrefix + connID }

func objectKey(connID string, ref ObjectRef) string {
	return objectPrefix + connID + sep + ref.Namespace + sep + ref.Kind + sep + ref.Name
}

func connObjectPrefix(connID string) string { return objectPrefix + connID + sep }

// Service serves cached catalogs and object detail, collapsing concurrent
// catalog misses for the same connection into a single introspection.
type Service struct {
	cache Cache
	ttl   time.Duration
	group singleflight.Group
}

// NewService builds a Service over the given cache with the given entry TTL.
func NewService(cache Cache, ttl time.Duration) *Service {
	return &Service{cache: cache, ttl: ttl}
}

// Capabilities reports the driver's static kind catalog. It does not touch the
// target database, so it works even when the catalog cannot be introspected.
func (s *Service) Capabilities(intr Introspector) DriverCapabilities {
	return intr.Capabilities()
}

// Catalog returns the cached catalog for connID, or introspects on a miss.
func (s *Service) Catalog(ctx context.Context, connID string, intr Introspector) (*Catalog, error) {
	key := catalogKey(connID)
	if data, ok := s.cache.Get(key); ok {
		var c Catalog
		if err := gunzipJSON(data, &c); err == nil {
			return &c, nil
		}
	}
	v, err, _ := s.group.Do(key, func() (any, error) {
		cat, err := intr.IntrospectCatalog(ctx, CatalogOptions{})
		if err != nil {
			return nil, err
		}
		cat.Connection = connID
		if cat.GeneratedAt.IsZero() {
			cat.GeneratedAt = time.Now()
		}
		if data, err := gzipJSON(cat); err == nil {
			s.cache.Set(key, data, s.ttl)
		}
		return cat, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*Catalog), nil
}

// Objects returns detail for refs in request order, serving cached entries and
// introspecting only the missing refs in one driver call. Refs the driver does
// not return are omitted (partial success).
func (s *Service) Objects(ctx context.Context, connID string, refs []ObjectRef, intr Introspector) ([]Object, error) {
	found := make(map[ObjectRef]Object, len(refs))
	var missing []ObjectRef
	for _, ref := range refs {
		if data, ok := s.cache.Get(objectKey(connID, ref)); ok {
			var o Object
			if err := gunzipJSON(data, &o); err == nil {
				found[ref] = o
				continue
			}
		}
		missing = append(missing, ref)
	}
	if len(missing) > 0 {
		fetched, err := intr.IntrospectObjects(ctx, missing)
		if err != nil {
			return nil, err
		}
		for _, o := range fetched {
			if data, err := gzipJSON(o); err == nil {
				s.cache.Set(objectKey(connID, o.Ref), data, s.ttl)
			}
			found[o.Ref] = o
		}
	}
	out := make([]Object, 0, len(refs))
	for _, ref := range refs {
		if o, ok := found[ref]; ok {
			out = append(out, o)
		}
	}
	return out, nil
}

// RefreshObject drops one object's cached detail.
func (s *Service) RefreshObject(connID string, ref ObjectRef) {
	s.cache.Invalidate(objectKey(connID, ref))
}

// RefreshConnection drops the catalog and all object detail for the connection.
func (s *Service) RefreshConnection(connID string) {
	s.cache.Invalidate(catalogKey(connID))
	s.cache.InvalidatePrefix(connObjectPrefix(connID))
}

func gzipJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if err := json.NewEncoder(gz).Encode(v); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func gunzipJSON(data []byte, v any) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer gz.Close()
	raw, err := io.ReadAll(gz)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}
