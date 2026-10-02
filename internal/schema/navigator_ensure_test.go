package schema

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	metadata "github.com/sqlwarden/internal/engine/metadata"
)

func tablesDemand(schema string) metadata.Demand {
	return metadata.Demand{Parent: schemaPathOf("app", schema), Folder: "tables"}
}

func TestEnsureForCompletionWithoutSessionDoesNothing(t *testing.T) {
	cat := newFakeCatalog()
	n := newTestNavigator(nil)
	report := n.EnsureForCompletion(context.Background(), Connection{ID: 1}, cat.Tree(), nil, []metadata.Demand{tablesDemand("public")})
	if report != (EnsureReport{}) || len(cat.callLog()) != 0 {
		t.Fatalf("report = %+v calls %v", report, cat.callLog())
	}
}

func TestEnsureForCompletionCapsAndDeduplicates(t *testing.T) {
	cat := newFakeCatalog()
	var demands []metadata.Demand
	for i := range 10 {
		schema := fmt.Sprintf("s%d", i)
		cat.set(schemaPathOf("app", schema), "tables", metadata.Child{Kind: "table", Name: "t"})
		demands = append(demands, tablesDemand(schema), tablesDemand(schema))
	}
	n := newTestNavigator(nil)
	report := n.EnsureForCompletion(context.Background(), Connection{ID: 1}, cat.Tree(), cat, demands)
	if report.Started != 8 || report.Completed != 8 || report.Capped != 2 {
		t.Fatalf("report = %+v", report)
	}
	if got := len(cat.callLog()); got != 8 {
		t.Fatalf("calls = %d", got)
	}
}

func TestEnsureForCompletionUpsertsMemoryAndStore(t *testing.T) {
	cat := newFakeCatalog()
	cat.set(schemaPathOf("app", "public"), "tables", metadata.Child{Kind: "table", Name: "orders"})
	store := newFakeStore()
	n := newTestNavigator(store)
	conn := Connection{ID: 1, Persistent: true}
	n.EnsureForCompletion(context.Background(), conn, cat.Tree(), cat, []metadata.Demand{tablesDemand("public")})
	for name, nav := range map[string]*Navigator{"memory": n, "store": newTestNavigator(store)} {
		listing, err := nav.Children(context.Background(), conn, cat.Tree(), nil, schemaPathOf("app", "public"), "tables")
		if err != nil || !slices.Equal(itemNames(listing), []string{"orders"}) {
			t.Fatalf("%s: listing = %+v err %v", name, listing, err)
		}
	}
	again := n.EnsureForCompletion(context.Background(), conn, cat.Tree(), cat, []metadata.Demand{tablesDemand("public")})
	if again.Cached != 1 || again.Started != 0 {
		t.Fatalf("second report = %+v", again)
	}
}

func TestEnsureForCompletionJoinsInFlightLoad(t *testing.T) {
	cat := newFakeCatalog()
	cat.set(schemaPathOf("app", "public"), "tables", metadata.Child{Kind: "table", Name: "orders"})
	cat.entered = make(chan struct{}, 1)
	cat.release = make(chan struct{})
	n := newTestNavigator(nil)
	conn := Connection{ID: 1}
	expanded := make(chan error, 1)
	go func() {
		_, err := n.Children(context.Background(), conn, cat.Tree(), cat, schemaPathOf("app", "public"), "tables")
		expanded <- err
	}()
	<-cat.entered
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	report := n.EnsureForCompletion(ctx, conn, cat.Tree(), cat, []metadata.Demand{tablesDemand("public")})
	if report.Joined != 1 || report.Started != 0 || report.Completed != 0 {
		t.Fatalf("report = %+v", report)
	}
	close(cat.release)
	if err := <-expanded; err != nil {
		t.Fatal(err)
	}
	if got := cat.callLog(); !slices.Equal(got, []string{"tables:1"}) {
		t.Fatalf("calls = %v", got)
	}
}

func TestEnsureForCompletionContinuesDetachedAfterCancel(t *testing.T) {
	cat := newFakeCatalog()
	cat.set(schemaPathOf("app", "public"), "tables", metadata.Child{Kind: "table", Name: "orders"})
	cat.entered = make(chan struct{}, 1)
	cat.release = make(chan struct{})
	n := newTestNavigator(nil)
	conn := Connection{ID: 1}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan EnsureReport, 1)
	go func() {
		done <- n.EnsureForCompletion(ctx, conn, cat.Tree(), cat, []metadata.Demand{tablesDemand("public")})
	}()
	<-cat.entered
	cancel()
	if report := <-done; report.Started != 1 || report.Completed != 0 || report.Failed != 0 {
		t.Fatalf("report = %+v", report)
	}
	follower := make(chan Listing, 1)
	go func() {
		listing, _ := n.Children(context.Background(), conn, cat.Tree(), cat, schemaPathOf("app", "public"), "tables")
		follower <- listing
	}()
	close(cat.release)
	if got := itemNames(<-follower); !slices.Equal(got, []string{"orders"}) {
		t.Fatalf("items = %v", got)
	}
	if got := cat.callLog(); !slices.Equal(got, []string{"tables:1"}) {
		t.Fatalf("calls = %v (detached load must be joined, not repeated)", got)
	}
}

func TestEnsureForCompletionStartsNothingAfterCancel(t *testing.T) {
	cat := newFakeCatalog()
	n := newTestNavigator(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	report := n.EnsureForCompletion(ctx, Connection{ID: 1}, cat.Tree(), cat, []metadata.Demand{tablesDemand("public")})
	if report.Started != 0 || len(cat.callLog()) != 0 {
		t.Fatalf("report = %+v calls %v", report, cat.callLog())
	}
}

func TestEnsureForCompletionReportsFailureWithoutError(t *testing.T) {
	cat := newFakeCatalog()
	cat.fail = errors.New("permission denied")
	n := newTestNavigator(nil)
	report := n.EnsureForCompletion(context.Background(), Connection{ID: 1}, cat.Tree(), cat, []metadata.Demand{tablesDemand("public")})
	if report.Started != 1 || report.Failed != 1 || report.Completed != 0 {
		t.Fatalf("report = %+v", report)
	}
}

func TestEnsureForCompletionSkipsUnknownFolder(t *testing.T) {
	cat := newFakeCatalog()
	n := newTestNavigator(nil)
	report := n.EnsureForCompletion(context.Background(), Connection{ID: 1}, cat.Tree(), cat,
		[]metadata.Demand{{Parent: schemaPathOf("app", "public"), Folder: "nope"}})
	if report != (EnsureReport{}) || len(cat.callLog()) != 0 {
		t.Fatalf("report = %+v", report)
	}
}

func TestEnsureForCompletionLogsFailedLoadsAtDebugOnly(t *testing.T) {
	cat := newFakeCatalog()
	cat.fail = errors.New("permission denied")
	var buf bytes.Buffer
	n := newTestNavigator(nil)
	n.logger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	n.EnsureForCompletion(context.Background(), Connection{ID: 1}, cat.Tree(), cat, []metadata.Demand{tablesDemand("public")})
	out := buf.String()
	if !strings.Contains(out, "level=DEBUG") || !strings.Contains(out, "folder=tables") || strings.Contains(out, "level=WARN") {
		t.Fatalf("log = %q", out)
	}
}
