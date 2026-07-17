package extension

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/sqlwarden/internal/license"
)

type trackingCloser struct{ closed bool }

func (c *trackingCloser) Close() error {
	c.closed = true
	return nil
}

func TestRegistryValidatesModuleIdentityAndLicenseProvider(t *testing.T) {
	tests := []struct {
		name    string
		modules []Module
	}{
		{name: "invalid name", modules: []Module{{Name: "Bad Name"}}},
		{name: "duplicate name", modules: []Module{{Name: "one"}, {Name: "one"}}},
		{name: "multiple license providers", modules: []Module{
			{Name: "one", LicenseFactory: func(context.Context, BootstrapDeps) (license.Service, error) { return nil, nil }},
			{Name: "two", LicenseFactory: func(context.Context, BootstrapDeps) (license.Service, error) { return nil, nil }},
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
			Scope:   RoutePublic,
			Prefix:  "/other/path",
			Feature: "feature",
			Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		}}}, nil
	}})
	if _, err := r.Start(context.Background(), RuntimeDeps{}); err == nil {
		t.Fatal("expected unsafe route namespace to be rejected")
	}
}
