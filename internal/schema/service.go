// Package schema provides the application schema inspection service used by HTTP
// handlers. It owns caching, refresh, singleflight, logging, and serialization
// policy for schema metadata returned by database engines.
package schema

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"time"

	"github.com/sqlwarden/internal/cache"
	schemameta "github.com/sqlwarden/internal/dbengine/schema"
	"golang.org/x/sync/singleflight"
)

const (
	catalogPrefix = "cat:"
	objectPrefix  = "obj:"
	sep           = "\x00"
)

func catalogKey(connID string) string { return catalogPrefix + connID }

func objectKey(connID string, ref schemameta.ObjectRef) string {
	return objectPrefix + connID + sep + ref.Namespace + sep + ref.Kind + sep + ref.Name
}

func connObjectPrefix(connID string) string { return objectPrefix + connID + sep }

// Service serves cached catalogs and object detail, collapsing concurrent
// catalog misses for the same connection into a single inspection.
type Service struct {
	cache  cache.Cache
	ttl    time.Duration
	group  singleflight.Group
	logger *slog.Logger
}

// NewService builds a Service over the given cache with the given entry TTL.
func NewService(c cache.Cache, ttl time.Duration) *Service {
	return NewServiceWithLogger(c, ttl, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// NewServiceWithLogger builds a Service and emits structured inspection
// events. Logs intentionally include counts and kinds, but not object names or
// source bodies, because schema names can be sensitive in enterprise targets.
func NewServiceWithLogger(c cache.Cache, ttl time.Duration, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Service{cache: c, ttl: ttl, logger: logger}
}

// Spec reports the driver's static schema object catalog. It does not touch the
// target database, so it works even when the catalog cannot be inspected.
func (s *Service) Spec(intr schemameta.SchemaInspector) schemameta.SchemaSpec {
	spec := intr.SchemaSpec()
	s.logger.Debug("schema spec resolved",
		slog.Group("schema",
			"operation", "schema_spec",
			"dialect", spec.Dialect,
			"kinds", len(spec.Kinds),
		),
	)
	return spec
}

// Catalog returns the cached catalog for connID, or inspects on a miss.
func (s *Service) Catalog(ctx context.Context, connID string, intr schemameta.SchemaInspector) (*schemameta.Catalog, error) {
	key := catalogKey(connID)
	start := time.Now()
	if data, ok := s.cache.Get(key); ok {
		var c schemameta.Catalog
		decodeErr := gunzipJSON(data, &c)
		if decodeErr == nil {
			s.logger.Debug("schema catalog cache hit",
				slog.Group("schema",
					"operation", "catalog",
					"conn_id", connID,
					"cache", "hit",
					"dialect", c.Dialect,
					"namespaces", len(c.Namespaces),
					"objects", countCatalogObjects(&c),
					"duration", time.Since(start).String(),
				),
			)
			return &c, nil
		}
		s.logger.Warn("schema catalog cache entry unreadable",
			slog.Group("schema",
				"operation", "catalog",
				"conn_id", connID,
				"cache", "corrupt",
			),
			"error", decodeErr,
		)
	}

	s.logger.Debug("schema catalog cache miss",
		slog.Group("schema",
			"operation", "catalog",
			"conn_id", connID,
			"cache", "miss",
		),
	)

	v, err, shared := s.group.Do(key, func() (any, error) {
		inspectStart := time.Now()
		cat, err := intr.InspectCatalog(ctx, schemameta.CatalogOptions{})
		if err != nil {
			s.logger.Warn("schema catalog inspection failed",
				slog.Group("schema",
					"operation", "catalog",
					"conn_id", connID,
					"duration", time.Since(inspectStart).String(),
				),
				"error", err,
			)
			return nil, err
		}
		cat.Connection = connID
		if cat.GeneratedAt.IsZero() {
			cat.GeneratedAt = time.Now()
		}
		if data, err := gzipJSON(cat); err == nil {
			s.cache.Set(key, data, s.ttl)
		} else {
			s.logger.Warn("schema catalog cache encode failed",
				slog.Group("schema",
					"operation", "catalog",
					"conn_id", connID,
				),
				"error", err,
			)
		}
		s.logger.Info("schema catalog inspected",
			slog.Group("schema",
				"operation", "catalog",
				"conn_id", connID,
				"dialect", cat.Dialect,
				"namespaces", len(cat.Namespaces),
				"objects", countCatalogObjects(cat),
				"duration", time.Since(inspectStart).String(),
			),
		)
		return cat, nil
	})
	if err != nil {
		return nil, err
	}
	if shared {
		cat := v.(*schemameta.Catalog)
		s.logger.Debug("schema catalog singleflight shared",
			slog.Group("schema",
				"operation", "catalog",
				"conn_id", connID,
				"dialect", cat.Dialect,
				"duration", time.Since(start).String(),
			),
		)
	}
	return v.(*schemameta.Catalog), nil
}

// Objects returns detail for refs in request order, serving cached entries and
// inspecting only the missing refs in one driver call. Refs the driver does not
// return are omitted (partial success).
func (s *Service) Objects(ctx context.Context, connID string, refs []schemameta.ObjectRef, intr schemameta.SchemaInspector) ([]schemameta.Object, error) {
	start := time.Now()
	found := make(map[schemameta.ObjectRef]schemameta.Object, len(refs))
	var missing []schemameta.ObjectRef
	for _, ref := range refs {
		if data, ok := s.cache.Get(objectKey(connID, ref)); ok {
			var o schemameta.Object
			decodeErr := gunzipJSON(data, &o)
			if decodeErr == nil {
				found[ref] = o
				continue
			}
			s.logger.Warn("schema object cache entry unreadable",
				slog.Group("schema",
					"operation", "objects",
					"conn_id", connID,
					"kind", ref.Kind,
					"cache", "corrupt",
				),
				"error", decodeErr,
			)
		}
		missing = append(missing, ref)
	}
	s.logger.Debug("schema object detail cache checked",
		slog.Group("schema",
			"operation", "objects",
			"conn_id", connID,
			"requested", len(refs),
			"cache_hits", len(refs)-len(missing),
			"cache_misses", len(missing),
		),
		"kinds", objectRefKindCounts(refs),
	)
	if len(missing) > 0 {
		inspectStart := time.Now()
		fetched, err := intr.InspectObjects(ctx, missing)
		if err != nil {
			s.logger.Warn("schema object detail inspection failed",
				slog.Group("schema",
					"operation", "objects",
					"conn_id", connID,
					"requested", len(refs),
					"missing", len(missing),
					"duration", time.Since(inspectStart).String(),
				),
				"kinds", objectRefKindCounts(missing),
				"error", err,
			)
			return nil, err
		}
		for _, o := range fetched {
			if data, err := gzipJSON(o); err == nil {
				s.cache.Set(objectKey(connID, o.Ref), data, s.ttl)
			} else {
				s.logger.Warn("schema object detail cache encode failed",
					slog.Group("schema",
						"operation", "objects",
						"conn_id", connID,
						"kind", o.Ref.Kind,
					),
					"error", err,
				)
			}
			found[o.Ref] = o
		}
		s.logger.Info("schema object details inspected",
			slog.Group("schema",
				"operation", "objects",
				"conn_id", connID,
				"requested", len(refs),
				"cache_misses", len(missing),
				"fetched", len(fetched),
				"duration", time.Since(inspectStart).String(),
			),
			"kinds", objectRefKindCounts(missing),
		)
	}
	out := make([]schemameta.Object, 0, len(refs))
	for _, ref := range refs {
		if o, ok := found[ref]; ok {
			out = append(out, o)
		}
	}
	s.logger.Debug("schema object detail response assembled",
		slog.Group("schema",
			"operation", "objects",
			"conn_id", connID,
			"requested", len(refs),
			"returned", len(out),
			"duration", time.Since(start).String(),
		),
	)
	return out, nil
}

// RefreshObject drops one object's cached detail.
func (s *Service) RefreshObject(connID string, ref schemameta.ObjectRef) {
	s.cache.Invalidate(objectKey(connID, ref))
	s.logger.Info("schema object cache invalidated",
		slog.Group("schema",
			"operation", "refresh_object",
			"conn_id", connID,
			"kind", ref.Kind,
		),
	)
}

// RefreshConnection drops the catalog and all object detail for the connection.
func (s *Service) RefreshConnection(connID string) {
	s.cache.Invalidate(catalogKey(connID))
	s.cache.InvalidatePrefix(connObjectPrefix(connID))
	s.logger.Info("schema connection cache invalidated",
		slog.Group("schema",
			"operation", "refresh_connection",
			"conn_id", connID,
		),
	)
}

func countCatalogObjects(cat *schemameta.Catalog) int {
	if cat == nil {
		return 0
	}
	total := 0
	for _, ns := range cat.Namespaces {
		for _, group := range ns.Groups {
			total += len(group.Objects)
		}
	}
	return total
}

func objectRefKindCounts(refs []schemameta.ObjectRef) map[string]int {
	counts := make(map[string]int)
	for _, ref := range refs {
		counts[ref.Kind]++
	}
	return counts
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
