package access

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/sqlwarden/internal/platform/invalidate"
	"github.com/uptrace/bun"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

// droppingBus accepts every publish and never delivers it.
type droppingBus struct{ published []string }

func (b *droppingBus) Publish(_ context.Context, _ invalidate.Topic, key string) error {
	b.published = append(b.published, key)
	return nil
}

func (b *droppingBus) Subscribe(invalidate.Topic, func(string)) func() { return func() {} }

// failingBus delivers nothing and reports every publish as failed.
type failingBus struct{ droppingBus }

func (b *failingBus) Publish(ctx context.Context, topic invalidate.Topic, key string) error {
	_ = b.droppingBus.Publish(ctx, topic, key)
	return errors.New("transport down")
}

func fillCache(c *MemoryAuthorizationCache) {
	c.SetOrgPolicy(1, &OrgPolicy{})
	c.SetPrincipals(1, 2, Principals{OrgID: 1, OrgMember: true})
	c.SetAncestry("connection", 3, []AncestorLevel{{ResourceType: "workspace", ResourceID: 4}})
}

func cacheHas(c *MemoryAuthorizationCache) (policy, principals, ancestry bool) {
	_, policy = c.GetOrgPolicy(1)
	_, principals = c.GetPrincipals(1, 2)
	_, ancestry = c.GetAncestry("connection", 3)
	return policy, principals, ancestry
}

func TestAuthorizationCacheKeysRoundTrip(t *testing.T) {
	cases := map[string]string{
		orgPolicyKey(1):                  "org:1",
		principalsKey(1, 2):              "principals:1:2",
		ancestryKey("connection", 3):     "ancestry:connection:3",
		ancestryKey("odd:type", 9223372): "ancestry:odd:type:9223372",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("key %q, want %q", got, want)
		}
	}

	c := newMemoryAuthorizationCache(nil, discardLogger(), time.Now)
	c.SetAncestry("odd:type", 9223372, nil)
	c.applyInvalidation(ancestryKey("odd:type", 9223372))
	if _, ok := c.GetAncestry("odd:type", 9223372); ok {
		t.Fatal("ancestry key with a colon in its type was not applied")
	}
}

func TestAuthorizationCacheInvalidationReachesEveryCacheOnTheBus(t *testing.T) {
	bus := invalidate.NewMemory(discardLogger())
	a := newMemoryAuthorizationCache(bus, discardLogger(), time.Now)
	b := newMemoryAuthorizationCache(bus, discardLogger(), time.Now)

	for _, tc := range []struct {
		name       string
		invalidate func(*MemoryAuthorizationCache)
		want       [3]bool
	}{
		{"org policy", func(c *MemoryAuthorizationCache) { c.InvalidateOrgPolicy(1) }, [3]bool{false, true, true}},
		{"principals", func(c *MemoryAuthorizationCache) { c.InvalidatePrincipals(1, 2) }, [3]bool{true, false, true}},
		{"ancestry", func(c *MemoryAuthorizationCache) { c.InvalidateAncestry("connection", 3) }, [3]bool{true, true, false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fillCache(a)
			fillCache(b)
			tc.invalidate(a)
			for name, c := range map[string]*MemoryAuthorizationCache{"publisher": a, "peer": b} {
				p, pr, an := cacheHas(c)
				if got := [3]bool{p, pr, an}; got != tc.want {
					t.Errorf("%s cache has %v, want %v", name, got, tc.want)
				}
			}
		})
	}
}

func TestAuthorizationCacheDroppedInvalidationExpiresAtTTL(t *testing.T) {
	bus := &droppingBus{}
	clock := &fakeClock{t: time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)}
	publisher := newMemoryAuthorizationCache(bus, discardLogger(), clock.now)
	peer := newMemoryAuthorizationCache(bus, discardLogger(), clock.now)
	fillCache(peer)

	publisher.InvalidateOrgPolicy(1)
	publisher.InvalidatePrincipals(1, 2)
	publisher.InvalidateAncestry("connection", 3)
	if len(bus.published) != 3 {
		t.Fatalf("published %v, want three keys", bus.published)
	}

	clock.t = clock.t.Add(authorizationCacheTTL)
	if p, pr, an := cacheHas(peer); !p || !pr || !an {
		t.Fatalf("peer lost entries before the TTL: %v %v %v", p, pr, an)
	}
	clock.t = clock.t.Add(time.Nanosecond)
	if p, pr, an := cacheHas(peer); p || pr || an {
		t.Fatalf("peer kept entries past the TTL: %v %v %v", p, pr, an)
	}
}

func TestAuthorizationCachePublishFailureStillDeletesLocally(t *testing.T) {
	bus := &failingBus{}
	c := newMemoryAuthorizationCache(bus, discardLogger(), time.Now)
	fillCache(c)

	c.InvalidateOrgPolicy(1)
	c.InvalidatePrincipals(1, 2)
	c.InvalidateAncestry("connection", 3)

	if p, pr, an := cacheHas(c); p || pr || an {
		t.Fatalf("entries survived a failed publish: %v %v %v", p, pr, an)
	}
	if len(bus.published) != 3 {
		t.Fatalf("published %v, want three attempts", bus.published)
	}
}

func TestAuthorizationCacheUnrecognizedKeyClearsEverything(t *testing.T) {
	bus := invalidate.NewMemory(discardLogger())
	c := newMemoryAuthorizationCache(bus, discardLogger(), time.Now)
	for _, key := range []string{"", "org:x", "principals:1", "ancestry:7", "future:1"} {
		fillCache(c)
		if err := bus.Publish(context.Background(), invalidate.TopicAuthz, key); err != nil {
			t.Fatal(err)
		}
		if p, pr, an := cacheHas(c); p || pr || an {
			t.Errorf("key %q left entries: %v %v %v", key, p, pr, an)
		}
	}
}

func TestAuthorizationCacheWithoutBusInvalidatesLocally(t *testing.T) {
	c := NewMemoryAuthorizationCache()
	fillCache(c)
	c.InvalidateOrgPolicy(1)
	c.InvalidatePrincipals(1, 2)
	c.InvalidateAncestry("connection", 3)
	if p, pr, an := cacheHas(c); p || pr || an {
		t.Fatalf("entries survived invalidation: %v %v %v", p, pr, an)
	}
}

func TestNewWithInvalidatorSubscribesTheEnforcerCache(t *testing.T) {
	bus := invalidate.NewMemory(discardLogger())
	e, err := New(new(bun.DB), WithInvalidator(bus), WithLogger(discardLogger()))
	if err != nil {
		t.Fatal(err)
	}
	c := e.cache.(*MemoryAuthorizationCache)
	fillCache(c)

	peer := newMemoryAuthorizationCache(bus, discardLogger(), time.Now)
	peer.InvalidateOrgPolicy(1)

	if _, ok := c.GetOrgPolicy(1); ok {
		t.Fatal("enforcer cache ignored an invalidation published on its bus")
	}
}
