package execution_test

import (
	"context"
	"testing"
	"time"

	"github.com/sqlwarden/internal/execution"
)

func TestStaticSessionDirectoryAlwaysRoutesToConnector(t *testing.T) {
	directory := execution.NewStaticSessionDirectory(" connector.internal:6021 ")
	ctx := context.Background()
	for _, handle := range []execution.SessionHandle{"", "one", "two"} {
		record, found, err := directory.Get(ctx, handle)
		if err != nil || !found || record.Handle != handle || record.RoutingAddress != "connector.internal:6021" {
			t.Fatalf("Get(%q) = (%+v, %v, %v)", handle, record, found, err)
		}
	}
	if err := directory.Put(ctx, execution.DirectoryRecord{}); err != nil {
		t.Fatal(err)
	}
	if err := directory.Renew(ctx, "one", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := directory.Delete(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	if _, found, err := directory.Get(ctx, "one"); err != nil || !found {
		t.Fatalf("mutations changed static route: found=%v err=%v", found, err)
	}
}

func TestStaticSessionDirectoryWithoutAddressDoesNotResolve(t *testing.T) {
	directory := execution.NewStaticSessionDirectory(" ")
	if _, found, err := directory.Get(context.Background(), "handle"); err != nil || found {
		t.Fatalf("Get() = (found=%v, err=%v), want unresolved", found, err)
	}
}
