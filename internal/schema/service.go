package schema

import (
	"context"
	"time"

	"golang.org/x/sync/singleflight"
)

// Service returns connection schemas, serving cached snapshots and collapsing
// concurrent cache misses for the same connection into a single introspection.
type Service struct {
	cache Cache
	ttl   time.Duration
	group singleflight.Group
}

// NewService builds a Service over the given cache with the given snapshot TTL.
func NewService(cache Cache, ttl time.Duration) *Service {
	return &Service{cache: cache, ttl: ttl}
}

// Get returns the cached schema for connID, or introspects via intr on a miss
// and caches the result connection-wide. Concurrent misses for the same connID
// run exactly one introspection.
func (s *Service) Get(ctx context.Context, connID string, intr Introspector) (*Schema, error) {
	if cached, ok := s.cache.Get(connID); ok {
		return cached, nil
	}
	return s.load(ctx, connID, intr)
}

// Refresh discards any cached snapshot for connID and re-introspects.
func (s *Service) Refresh(ctx context.Context, connID string, intr Introspector) (*Schema, error) {
	s.cache.Invalidate(connID)
	return s.load(ctx, connID, intr)
}

func (s *Service) load(ctx context.Context, connID string, intr Introspector) (*Schema, error) {
	v, err, _ := s.group.Do(connID, func() (any, error) {
		snap, err := intr.Introspect(ctx, IntrospectOptions{})
		if err != nil {
			return nil, err
		}
		snap.Connection = connID
		if snap.GeneratedAt.IsZero() {
			snap.GeneratedAt = time.Now()
		}
		s.cache.Set(connID, snap, s.ttl)
		return snap, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*Schema), nil
}
