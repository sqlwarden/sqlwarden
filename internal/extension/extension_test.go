package extension

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/capability"
)

type trackingCloser struct{ closed bool }

func (c *trackingCloser) Close() error {
	c.closed = true
	return nil
}

func TestRegistryValidatesModuleIdentityAndCapabilityFactory(t *testing.T) {
	tests := []struct {
		name    string
		modules []Module
	}{
		{name: "invalid name", modules: []Module{{Name: "Bad Name"}}},
		{name: "name too long for migration table", modules: []Module{{Name: "a" + strings.Repeat("b", maxModuleNameLength)}}},
		{name: "duplicate name", modules: []Module{{Name: "one"}, {Name: "one"}}},
		{name: "multiple capability providers", modules: []Module{
			{Name: "one", CapabilityFactory: func(context.Context, BootstrapDeps) (capability.Gate, error) { return nil, nil }},
			{Name: "two", CapabilityFactory: func(context.Context, BootstrapDeps) (capability.Gate, error) { return nil, nil }},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRegistry()
			r.Add(tt.modules...)
			if err := r.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestRegistryRejectsRoutesThatCollideAfterMounting(t *testing.T) {
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	r := NewRegistry()
	r.Add(Module{Name: "one", Start: func(context.Context, RuntimeDeps) (Contributions, error) {
		return Contributions{Routes: []Route{
			{Scope: RoutePublic, Prefix: "/one/callback", Access: RouteAccessCapability, Capability: "feature", Handler: handler},
			{Scope: RouteAccount, Prefix: "/one/callback", Access: RouteAccessCapability, Capability: "feature", Handler: handler},
		}}, nil
	}})
	if _, err := r.Start(context.Background(), RuntimeDeps{}); err == nil {
		t.Fatal("expected routes sharing the API mount point to be rejected")
	}
}

func TestRegistryStartClosesEarlierModulesWhenLaterModuleFails(t *testing.T) {
	closer := &trackingCloser{}
	r := NewRegistry()
	r.Add(
		Module{Name: "one", Start: func(context.Context, RuntimeDeps) (Contributions, error) {
			return Contributions{Closers: []io.Closer{closer}}, nil
		}},
		Module{Name: "two", Start: func(context.Context, RuntimeDeps) (Contributions, error) {
			return Contributions{}, errors.New("boom")
		}},
	)
	if _, err := r.Start(context.Background(), RuntimeDeps{}); err == nil {
		t.Fatal("expected startup error")
	}
	if !closer.closed {
		t.Fatal("expected previously started module to be closed")
	}
}

func TestRegistryRejectsUnsafeContributions(t *testing.T) {
	r := NewRegistry()
	r.Add(Module{Name: "one", Start: func(context.Context, RuntimeDeps) (Contributions, error) {
		return Contributions{Routes: []Route{{
			Scope:      RoutePublic,
			Prefix:     "/other/path",
			Access:     RouteAccessCapability,
			Capability: "feature",
			Handler:    http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		}}}, nil
	}})
	if _, err := r.Start(context.Background(), RuntimeDeps{}); err == nil {
		t.Fatal("expected unsafe route namespace to be rejected")
	}
}

func TestRegistryValidatesRouteAccessPolicy(t *testing.T) {
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	tests := []Route{
		{Scope: RoutePublic, Prefix: "/one/missing", Handler: handler},
		{Scope: RoutePublic, Prefix: "/one/capability", Access: RouteAccessCapability, Handler: handler},
		{Scope: RoutePublic, Prefix: "/one/always", Access: RouteAccessAlways, Capability: "feature", Handler: handler},
	}
	for _, route := range tests {
		r := NewRegistry()
		r.Add(Module{Name: "one", Start: func(context.Context, RuntimeDeps) (Contributions, error) {
			return Contributions{Routes: []Route{route}}, nil
		}})
		if _, err := r.Start(context.Background(), RuntimeDeps{}); err == nil {
			t.Fatalf("expected route %+v to be rejected", route)
		}
	}
}
